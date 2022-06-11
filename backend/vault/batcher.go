package vault

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path"
	"strconv"
	"sync"
	"time"

	"github.com/rclone/rclone/backend/vault/api"
	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/lib/atexit"
	"github.com/rclone/rclone/lib/rest"
	"github.com/schollz/progressbar/v3"
)

// batcher is used to group mass upload for a deposit.
type batcher struct {
	fs           *Fs             // fs.root will be the parent collection or folder
	atexit       atexit.FnHandle // callback
	parent       *api.TreeNode   // resolved and possibly new parent treenode
	once         sync.Once       // only batch wrap up once
	mu           sync.Mutex      // protect items
	items        []*batchItem    // file metadata and content for deposit items
	showProgress bool            // show progress bar
}

// newBatcher creates a new batcher, which will execute most code at rclone
// exit time.
func newBatcher(ctx context.Context, f *Fs) (*batcher, error) {
	t, err := f.api.ResolvePath(f.root)
	if err != nil {
		if err == fs.ErrorObjectNotFound {
			// TODO: We only want this, if we actually have a Put request (not any, as currently).
			if err = f.mkdir(ctx, f.root); err != nil {
				return nil, err
			}
			if t, err = f.api.ResolvePath(f.root); err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}
	b := &batcher{
		fs:     f,
		parent: t,
	}
	b.atexit = atexit.Register(b.Shutdown)
	// When interrupted, we may have items in the batch, clean these up before
	// the shutdown handler runs.
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		for _ = range c {
			b.mu.Lock()
			if len(b.items) > 0 {
				fs.Logf(b, "ignoring %d batch entries", len(b.items))
			}
			b.items = b.items[:0]
			b.mu.Unlock()
		}
	}()
	fs.Debugf(b, "initialized batcher")
	return b, nil
}

// batchItem for Put and Update requests, basically capturing argument.
type batchItem struct {
	root     string // the fs root
	filename string // temporary file with content
	src      fs.ObjectInfo
	options  []fs.OpenOption
}

// ToFile turns a batch item into a File for a deposit request.
func (item *batchItem) ToFile(ctx context.Context) *api.File {
	var (
		randInt        = 1_000_000_000 + rand.Intn(8_999_999_999)
		randSuffix     = fmt.Sprintf("%s-%d", time.Now().Format("20060102030405"), randInt)
		flowIdentifier = fmt.Sprintf("rclone-vault-flow-%s", randSuffix)
	)
	return &api.File{
		Name:                 path.Base(item.src.Remote()),
		FlowIdentifier:       flowIdentifier,
		RelativePath:         item.src.Remote(),
		Size:                 item.src.Size(),
		PreDepositModifiedAt: item.src.ModTime(ctx).Format("2006-01-02T03:04:05.000Z"),
		Type:                 item.contentType(),
	}
}

// contentType tries to sniff the content type, or returns the empty string.
func (item *batchItem) contentType() string {
	f, err := os.Open(item.filename)
	if err != nil {
		return ""
	}
	defer f.Close()
	buf := make([]byte, 512)
	if _, err := f.Read(buf); err != nil {
		return ""
	}
	switch v := http.DetectContentType(buf); v {
	case "application/octet-stream":
		// DetectContentType always returns a valid MIME type: if it cannot
		// determine a more specific one, it returns
		// "application/octet-stream".
		return ""
	default:
		return v
	}
}

func (b *batcher) String() string {
	return "vault batcher"
}

// Add a single batch item.
func (b *batcher) Add(item *batchItem) {
	b.mu.Lock()
	b.items = append(b.items, item)
	b.mu.Unlock()
}

// Shutdown creates a new deposit request for all batch items and uploads them.
// This is the one of the last things rclone run before exiting. There is no
// error to return.
func (b *batcher) Shutdown() {
	fs.Debugf(b, "shutdown started")
	b.once.Do(func() {
		atexit.Unregister(b.atexit)
		signal.Reset(os.Interrupt)
		if len(b.items) == 0 {
			fs.Debugf(b, "nothing to deposit")
			return
		}
		var (
			// We do not want to be cancelled here; or if, we want to set our
			// own timeout for deposit uploads.
			ctx             = context.Background()
			totalSize int64 = 0
			files     []*api.File
			bar       *progressbar.ProgressBar
		)
		fs.Debugf(b, "preparing %d files for deposit", len(b.items))
		for _, item := range b.items {
			totalSize += item.src.Size()
			files = append(files, item.ToFile(ctx))
		}
		rdr := &api.RegisterDepositRequest{
			TotalSize: totalSize,
			Files:     files,
		}
		switch {
		case b.parent.NodeType == "COLLECTION":
			c, err := b.fs.api.TreeNodeToCollection(b.parent)
			if err != nil {
				fs.LogLevelPrintf(fs.LogLevelError, b, "failed to resolve treenode to collection")
				return
			}
			rdr.CollectionId = c.Identifier()
		case b.parent.NodeType == "FOLDER":
			rdr.ParentNodeId = b.parent.Id
		}
		depositId, err := b.fs.api.RegisterDeposit(ctx, rdr)
		if err != nil {
			fs.LogLevelPrintf(fs.LogLevelError, b, "deposit failed: %v", err)
			return
		}
		fs.Debugf(b, "created deposit %v", depositId)
		if b.showProgress {
			bar = progressbar.DefaultBytes(totalSize, "<5>NOTICE: depositing")
		}
		for i, item := range b.items {
			// Upload file with a single chunk. First issue a GET, if that is a
			// 204 then a POST.
			fi, err := os.Stat(item.filename)
			if err != nil {
				return
			}
			size := fi.Size()
			if b.showProgress {
				bar.Add(int(size))
			}
			params := url.Values{
				"depositId":            []string{strconv.Itoa(int(depositId))},
				"flowChunkNumber":      []string{"1"},
				"flowChunkSize":        []string{strconv.Itoa(int(size))},
				"flowCurrentChunkSize": []string{strconv.Itoa(int(size))},
				"flowFilename":         []string{files[i].Name},
				"flowIdentifier":       []string{files[i].FlowIdentifier},
				"flowRelativePath":     []string{files[i].RelativePath},
				"flowTotalChunks":      []string{"1"},
				"flowTotalSize":        []string{strconv.Itoa(int(size))},
				"upload_token":         []string{"my_token"},
			}
			opts := rest.Opts{
				Method:     "GET",
				Path:       "/flow_chunk",
				Parameters: params,
			}
			resp, err := b.fs.api.Call(ctx, &opts)
			if err != nil {
				fs.LogLevelPrintf(fs.LogLevelError, b, "call failed: %v", err)
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != 204 {
				fs.LogLevelPrintf(fs.LogLevelError, b, "expected HTTP 204, got %v", resp.StatusCode)
				return
			}
			itemf, err := os.Open(item.filename)
			if err != nil {
				fs.LogLevelPrintf(fs.LogLevelError, b, "failed to open temporary file: %v", err)
				return
			}
			opts = rest.Opts{
				Method:               "POST",
				Path:                 "/flow_chunk",
				MultipartParams:      params,
				ContentLength:        &size,
				MultipartContentName: "file",
				MultipartFileName:    path.Base(item.src.Remote()),
				Body:                 itemf,
			}
			resp, err = b.fs.api.CallJSON(ctx, &opts, nil, nil)
			if err != nil {
				fs.LogLevelPrintf(fs.LogLevelError, b, "upload failed: %v", err)
				return
			}
			if err := resp.Body.Close(); err != nil {
				fs.LogLevelPrintf(fs.LogLevelWarning, b, "body close: %v", err)
			}
			if err := itemf.Close(); err != nil {
				fs.LogLevelPrintf(fs.LogLevelWarning, b, "file close: %v", err)
			}
			if err := os.Remove(item.filename); err != nil {
				fs.LogLevelPrintf(fs.LogLevelWarning, b, "cleanup: %v", err)
			}
		}
		fs.Logf(b, "upload done, deposited %d item(s)", len(b.items))
	})
}
