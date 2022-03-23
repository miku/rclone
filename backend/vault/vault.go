// Package vault adds support for the Internet Archive Vault Digital
// Preservation System. Learn more at https://archive.org and
// https://archive-it.org/.
//
// This is very much exploratory at this point.
//
// Concepts in Vault
//
//     User -- Organization
//
//     Organization
//         Collection
//             File
//             Folder
//                 Folder
//                 File
//                 ...
//             ...
//         Collection
//             File
//             ...
//
//
// Possible config file
// --------------------
//
// [vault]
//
// username = admin
// password = ....
// endpoint = http://localhost:8000
//
// Config parameters are prefixed by the storage type, e.g. vault-username,
// vault-endpoint, ...
//
// User organization is 1:1 (optional). So given a user, we should have one
// treenode hierarchy rooted at the organization.
//
// Auth is currently handled via Remote User
// (https://docs.djangoproject.com/en/4.0/howto/auth-remote-user/).
//
//
// Some possible features
// ----------------------
//
// $ rclone link vault:/collection1/folder1/file1 -> http://archive.org/...
//
// Like: https://rclone.org/b2/#b2-and-rclone-link
//
//
// Options to consider
// -------------------
//
// A: Instead of implementing a specific vault handler, we could provide an S3
// like API in vault and let people use the existing S3 adapter, just changing
// URL and credentials.
//
// Object mapping:
//
//     Vault          S3
//
//     Organization   User
//     Collection     Bucket
//     Folder         Path
//     File           Path
//
// What is missing here is an S3 adapter in vault. Or an S3 component that
// talks to the Vault database.
//
// E.g. vault-s3-server -c vault-s3.ini
//
// Would need looks and would probably duplicate some effort across vault
// proper and s3 server.
//
// B: Create a separate "vault" provider.
//
// Could use the same terminology, e.g. organization, collection. Would talk
// directly to the API, less or no duplication.
//
// By means of rclone could still do things like rclone from S3 -> vault or
// vault -> gcp, etc.
//
// Overall more integrated, less dependency on an existing protocol. Could
// surface extra functionality unique to vault.
//
// Prototype:
//
// * [ ] read-only interface
// * [ ] add a new collection
// * [ ] add new folders and files
//
// Notes
// -----
//
// [ ] Fs
// [x]   Info
// [x]     Name() string
// [x]     Root() string
// [x]     String() string
// [x]     Precision() time.Duration
// [x]     Hashes ...
// [x]     Features() ...
// [x]   List
// [ ]   NewObject(..., remote string) (Object, error)
// [ ]   Put(..., in io.Reader, src ObjectInfo, options ...OpenOptions) (Object, error)
// [ ]   Mkdir(..., dir string) ...
// [ ]   Rmdir(..., dir string) ...
//
// [ ] Object
// [ ]   ObjectInfo
// [ ]     DirEntry
// [x]       String() string
// [x]       Remote() string
// [x]       ModTime() ...
// [x]       Size() ...
// [x]     Fs() Info
// [x]     Hash(...)
// [x]     Storable() bool
// [ ]   SetModTime(...) ...
// [ ]   Open(...) ...
// [ ]   Update(...)...
// [ ]   Remove(...) ...
//
// [ ] Directory
// [ ]   DirEntry
// [ ]     String() string
// [ ]     Remote() string
// [ ]     ModTime() ...
// [ ]     Size() ...
// [ ]  Items() int64
// [ ]  ID() string
//
// [ ] FullObject
// [ ]   Object
// [ ]   MimeTypes
// [ ]   IDer
// [ ]   ObjectUnWrapper
// [ ]   GetTierer
// [ ]   SetTierer
//
package vault

import (
	"context"
	"fmt"
	"io"
	"log"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/fs/config/configmap"
	"github.com/rclone/rclone/fs/config/configstruct"
	"github.com/rclone/rclone/fs/hash"
	"golang.org/x/sync/errgroup"
)

func init() {
	fs.Register(&fs.RegInfo{
		Name:        "vault",
		Description: "Internet Archive Vault Digital Preservation System",
		NewFs:       NewFs,
		Options: []fs.Option{
			{
				Name:    "username",
				Help:    "vault username (needs to belong to an organization to be usable)",
				Default: 0,
			},
			{
				Name:    "url",
				Help:    "vault API URL",
				Default: "http://localhost:8000/api",
			},
		},
	})
}

// Options for this backend.
type Options struct {
	Username string `config:"username"`
	Endpoint string `config:"url"`
}

func NewFs(ctx context.Context, name, root string, cm configmap.Mapper) (fs.Fs, error) {
	// log.Printf("NewFs(..., %s, %s, ...)", name, root)
	var (
		opts = new(Options)
		err  = configstruct.Set(cm, opts)
	)
	if err != nil {
		return nil, err
	}
	// TODO: if we have a file, we may want to set root to the containing folder?
	api := &Api{
		Endpoint: opts.Endpoint,
		Username: opts.Username,
	}
	// TODO: Do we need to change this here?
	// t, err := api.resolvePath(root)
	// if err != nil {
	// 	return nil, ErrPathNotFound
	// }
	// if t.NodeType == "FILE" {
	// 	log.Printf("file detected: %v", root)
	// 	root = path.Dir(root)
	// }
	return &Fs{
		name:      name,
		root:      root,
		opt:       *opts,
		api:       api,
		pathCache: make(map[string]string),
	}, nil
}

// Fs represents Vault as a file system.
type Fs struct {
	name      string            // just "vault"
	root      string            // the requested path
	opt       Options           // parsed config options
	api       *Api              // API wrapper
	pathCache map[string]string // a cache for the duration of an operation
}

type Folder struct {
	Object
}

// Items returns number of child items.
func (f *Folder) Items() int64 {
	items, err := f.Object.fs.api.List(f.Object.treeNode)
	if err != nil {
		return 0
	}
	return int64(len(items))
}

// ID return the treenode path, which is unique.
func (f *Folder) ID() string {
	return f.Object.treeNode.Path
}

// Object represents a vault object, which is defined by a treeNode. Implements
// various interfaces. Can represent organization, collection, folder or file.
// Not all operations will succeed on all node types.
type Object struct {
	fs       *Fs       // The Fs this object is part of
	treeNode *TreeNode // TreeNode object, contains additional metadata, like mod time or hash
}

// Fs returns the Fs this object belongs to.
func (o *Object) Fs() fs.Info {
	return o.fs
}

// Hash return a selected checksum.
func (o *Object) Hash(_ context.Context, ty hash.Type) (string, error) {
	log.Println("Hash")
	if o.treeNode == nil {
		return "", nil
	}
	switch ty {
	case hash.MD5:
		if v, ok := o.treeNode.Md5Sum.(string); ok {
			return v, nil
		}
	case hash.SHA1:
		if v, ok := o.treeNode.Sha1Sum.(string); ok {
			return v, nil
		}
	case hash.SHA256:
		if v, ok := o.treeNode.Sha256Sum.(string); ok {
			return v, nil
		}
	}
	return "", nil
}

// Storable, whether we can store data (TODO: look this up).
func (o *Object) Storable() bool {
	log.Println("Storable")
	return o.treeNode.NodeType == "FILE"
}

// String returns the name of the object.
func (o *Object) String() string {
	if o == nil {
		return "<nil>"
	}
	return o.treeNode.Name
}

// path turns the treenode path field into a filelike path structure using the
// name of the treenodes.
func (o *Object) path() string {
	var (
		labels   = o.treeNode.Path // "6", "6.22", "6.22.87", ...
		segments = strings.Split(labels, ".")
		names    = make([]string, len(segments)-1)
	)
	// TODO: we only need to cache segments individually, would speed up ops.
	if v, ok := o.fs.pathCache[labels]; ok {
		return v
	}
	// This signal broken internal data.
	if len(segments) == 0 {
		return ""
	}
	// We always skip the organization treenode.
	segments = segments[1:]
	// Request path segments in parallel and cache.
	g := new(errgroup.Group)
	for i, s := range segments {
		i, s := i, s
		g.Go(func() error {
			t, err := o.fs.api.GetTreeNode(s)
			if err != nil {
				return err
			}
			names[i] = t.Name
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return ""
	}
	v := "/" + strings.Join(names, "/")
	o.fs.pathCache[labels] = v
	return v
}

// relativePath returns the path relative to a given root, given as a string
// like "/a/b" etc.
func (o *Object) relativePath() string {
	var root, relpath string
	if root = o.fs.root; root == "" {
		root = "/"
	}
	relpath = strings.Replace(o.path(), root, "", 1)
	relpath = strings.TrimLeft(relpath, "/")
	return relpath
}

// Remote turns a treenode into a path. This can fail silently.
//
// TODO: The remote name needs to be relative to the Fs.root, not the absolute path.
// TODO: Can use TreeNode path to access treenodes faster and to remote root from path.
func (o *Object) Remote() string {
	return o.relativePath()
}

// ModTime returns the modification time of the object or epoch, if the time is not available.
func (o *Object) ModTime(_ context.Context) time.Time {
	epoch := time.Unix(0, 0)
	if o.treeNode == nil {
		return epoch
	}
	t, err := time.Parse("January 2, 2006 15:04:05 UTC", o.treeNode.ModifiedAt)
	if err == nil {
		return t
	}
	return epoch
}

// Size returns the size of an object.
func (o *Object) Size() int64 {
	if o.treeNode == nil {
		return 0
	}
	if o.treeNode.NodeType != "FILE" {
		return 0
	}
	switch v := o.treeNode.Size.(type) {
	case nil:
		return 0
	case int:
		return int64(v)
	case float64:
		return int64(v)
	case int64:
		return v
	default:
		return 0
	}
}

func (o *Object) SetModTime(ctx context.Context, t time.Time) error {
	log.Println("SetModTime")
	return nil
}

func (o *Object) Open(ctx context.Context, options ...fs.OpenOption) (io.ReadCloser, error) {
	log.Println("Open")
	return nil, nil
}

func (o *Object) Update(ctx context.Context, in io.Reader, src fs.ObjectInfo, options ...fs.OpenOption) error {
	log.Println("Update")
	return nil
}

func (o *Object) Remove(ctx context.Context) error {
	log.Println("Remove")
	return nil
}

// Name of the remote (as passed into NewFs)
func (f *Fs) Name() string {
	// log.Println("Name")
	return f.name
}

// Root of the remote (as passed into NewFs)
func (f *Fs) Root() string {
	// log.Println("Root")
	return f.root
}

// String returns a description of the Fs.
func (f *Fs) String() string {
	log.Println("String")
	return f.name
}

// Precision of the ModTimes in this Fs.
func (f *Fs) Precision() time.Duration {
	log.Println("Precision")
	return 1 * time.Second
}

// Returns the supported hash types of the filesystem
func (f *Fs) Hashes() hash.Set {
	log.Println("Hashes")
	return hash.Set(hash.None)
	// return hash.Set(hash.MD5 | hash.SHA1 | hash.SHA256)
}

// Features returns the optional features of this Fs.
func (f *Fs) Features() *fs.Features {
	// log.Println("Features")
	return &fs.Features{
		CaseInsensitive:         true,
		IsLocal:                 false,
		CanHaveEmptyDirectories: true,
		SlowHash:                true,
		BucketBased:             false,
		// DuplicateFiles:          false,
		// CanHaveEmptyDirectories: true,
		// BucketBased:             true,
		// BucketBasedRootOK:       true,
		// SetTier:                 false,
		// GetTier:                 false,
		// ServerSideAcrossConfigs: true,
	}
}

// func (f *Fs) listRoot(ctx context.Context) (entries fs.DirEntries, err error) {
// 	return nil, nil
// }

// List lists entries. If dir is a root, we would need to iterate over too many
// entries, basically all collection names and all top level items. We need a
// limit here, e.g. return the most recently modified 10000 items.
//
// If dir is an items, return all files in the item. If dir is a collection,
// return both files, collections and items.
// func (f *Fs) List(ctx context.Context, dir string) (entries fs.DirEntries, err error) {
// 	// List the objects and directories in dir into entries.  The
// 	// entries can be returned in any order but should be for a
// 	// complete directory.
// 	//
// 	// dir should be "" to list the root, and should not have
// 	// trailing slashes.
// 	//
// 	// This should return ErrDirNotFound if the directory isn't
// 	// found.
// 	if dir == "" {
// 		return f.listRoot(ctx)
// 	}
// 	return nil, nil
// }

// NewObject finds the Object at remote.  If it can't be found
// it returns the error ErrorObjectNotFound.
//
// If remote points to a directory then it should return
// ErrorIsDir if possible without doing any extra work,
// otherwise ErrorObjectNotFound.
// NewObject(ctx context.Context, remote string) (Object, error)

// Put in to the remote path with the modTime given of the given size
//
// When called from outside an Fs by rclone, src.Size() will always be >= 0.
// But for unknown-sized objects (indicated by src.Size() == -1), Put should either
// return an error or upload it properly (rather than e.g. calling panic).
//
// May create the object even if it returns an error - if so
// will return the object and the error, otherwise will return
// nil and the error
// Put(ctx context.Context, in io.Reader, src ObjectInfo, options ...OpenOption) (Object, error)

// Mkdir makes the directory (container, bucket)
//
// Shouldn't return an error if it already exists
// Mkdir(ctx context.Context, dir string) error

// Rmdir removes the directory (container, bucket) if empty
//
// Return an error if it doesn't exist or isn't empty
// Rmdir(ctx context.Context, dir string) error

// List the objects and directories in dir into entries.  The
// entries can be returned in any order but should be for a
// complete directory.
//
// dir should be "" to list the root, and should not have
// trailing slashes.
//
// This should return ErrDirNotFound if the directory isn't
// found.
func (f *Fs) List(ctx context.Context, dir string) (fs.DirEntries, error) {
	// log.Printf("List root=%v, dir=%v", f.root, dir)
	var (
		full    = filepath.Join(f.root, dir)
		nodes   []*TreeNode
		entries fs.DirEntries
		entry   fs.DirEntry
	)
	t, err := f.api.resolvePath(full)
	if err != nil {
		return nil, fs.ErrorDirNotFound
	}
	if t.NodeType == "FILE" {
		// Hack around listing a single file.
		// return nil, fs.ErrorDirNotFound
		f.root = path.Dir(f.root)
		entries = append(entries, &Object{
			fs:       f,
			treeNode: t,
		})
		return entries, nil
	}
	if nodes, err = f.api.List(t); err != nil {
		return nil, err
	}
	for _, t := range nodes {
		switch t.NodeType {
		case "FOLDER", "COLLECTION":
			entry = &Folder{
				Object: Object{
					fs:       f,
					treeNode: t,
				},
			}
		case "FILE":
			// return nil, fs.ErrorDirNotFound
			entry = &Object{
				fs:       f,
				treeNode: t,
			}
		default:
			return nil, fmt.Errorf("unexpected node type: %v", t.NodeType)
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// NewObject finds the Object at remote.  If it can't be found
// it returns the error ErrorObjectNotFound.
//
// If remote points to a directory then it should return
// ErrorIsDir if possible without doing any extra work,
// otherwise ErrorObjectNotFound.
func (f *Fs) NewObject(ctx context.Context, remote string) (fs.Object, error) {
	log.Println("NewObject")
	t, err := f.api.resolvePath(remote)
	if err != nil {
		return nil, fs.ErrorObjectNotFound
	}
	if t.NodeType == "FOLDER" || t.NodeType == "COLLECTION" {
		return nil, fs.ErrorIsDir
	}
	return &Object{
		fs:       f,
		treeNode: t,
	}, nil
}

// Put in to the remote path with the modTime given of the given size
//
// When called from outside an Fs by rclone, src.Size() will always be >= 0.
// But for unknown-sized objects (indicated by src.Size() == -1), Put should either
// return an error or upload it properly (rather than e.g. calling panic).
//
// May create the object even if it returns an error - if so
// will return the object and the error, otherwise will return
// nil and the error
func (f *Fs) Put(ctx context.Context, in io.Reader, src fs.ObjectInfo, options ...fs.OpenOption) (fs.Object, error) {
	log.Printf("Put: %v, %v, %v", f.root, src, options)

	// TODO: vault upload
	// 1 api/register_deposit
	// 2 api/flow_chunk
	return nil, nil
}

// Mkdir makes the directory (container, bucket)
//
// Shouldn't return an error if it already exists
func (f *Fs) Mkdir(ctx context.Context, dir string) error {
	log.Printf("mkdir root=%v dir=%v", f.root, dir)
	switch {
	case strings.Count(f.root, "/") == 1 && strings.HasPrefix(f.root, "/"):
		log.Printf("creating collection: %v", f.root[1:])
		err := f.api.CreateCollection(f.root[1:])
		if err != nil {
			return err
		}
	case strings.Count(f.root, "/") > 1:
		segments := strings.Split(f.root, "/")
		segments = segments[1:]
		log.Printf("creating folder under collection=%v folder=%v", segments[0], segments[1:])
	default:
		return fmt.Errorf("cannot create dir %v", f.root)
	}
	return nil
}

// Rmdir removes the directory (container, bucket) if empty
//
// Return an error if it doesn't exist or isn't empty
func (f *Fs) Rmdir(ctx context.Context, dir string) error {
	log.Printf("rmdir %v %v", f.root, dir)
	return nil
}

// Check the interfaces are satisfied
var (
	_ fs.Fs = &Fs{}
	// _ fs.Copier      = &Fs{}
	// _ fs.PutStreamer = &Fs{}
	// _ fs.ListRer     = &Fs{}
	_ fs.Object = &Object{}
	// _ fs.MimeTyper   = &Object{}
)
