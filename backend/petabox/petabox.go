package petabox

import (
	"context"
	"io"
	"log"
	"strings"
	"time"

	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/fs/config/configmap"
	"github.com/rclone/rclone/fs/hash"
)

func init() {
	fsi := &fs.RegInfo{
		Name:        "Internet Archive Petabox",
		Prefix:      "petabox",
		Description: "Internet Archive Petabox",
		NewFs:       NewFs,
		Options:     []fs.Option{},
	}
	fs.Register(fsi)
}

func NewFs(ctx context.Context, _, _ string, cm configmap.Mapper) (fs.Fs, error) {
	log.Println("hello, petabox!")
	return &DummyFs{}, nil
}

// DummyFs for poking around the rclone api.
type DummyFs struct{}

// Name of the remote (as passed into NewFs)
func (f *DummyFs) Name() string {
	return "petabox"
}

// Root of the remote (as passed into NewFs)
func (f *DummyFs) Root() string {
	return "/"
}

// String returns a description of the FS
func (f *DummyFs) String() string {
	return "ia petabox"
}

// Precision of the ModTimes in this Fs
func (f *DummyFs) Precision() time.Duration {
	return 1 * time.Second
}

// Returns the supported hash types of the filesystem
func (f *DummyFs) Hashes() hash.Set {
	return hash.NewHashSet(hash.MD5, hash.SHA1, hash.SHA256)
}

// Features returns the optional features of this Fs
func (f *DummyFs) Features() *fs.Features {
	return &fs.Features{
		CaseInsensitive:         true,
		CanHaveEmptyDirectories: true,
		IsLocal:                 false,
		SlowHash:                true,
	}
}

// DummyFile is an actual object. Embeds read-only object information as well.
type DummyFile struct {
	Name string
}

// SetModTime sets the metadata on the object to set the modification date
func (f *DummyFile) SetModTime(ctx context.Context, t time.Time) error {
	return nil
}

// Open opens the file for read.  Call Close() on the returned io.ReadCloser
func (f *DummyFile) Open(ctx context.Context, options ...fs.OpenOption) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("dummy content")), nil
}

// Update in to the object with the modTime given of the given size
//
// When called from outside an Fs by rclone, src.Size() will always be >= 0.
// But for unknown-sized objects (indicated by src.Size() == -1), Upload should either
// return an error or update the object properly (rather than e.g. calling panic).
func (f *DummyFile) Update(ctx context.Context, in io.Reader, src fs.ObjectInfo, options ...fs.OpenOption) error {
	return nil
}

// Removes this object
func (f *DummyFile) Remove(ctx context.Context) error {
	return nil
}

// dummyFile implementing DirEntry

func (f *DummyFile) String() string {
	return f.Name
}

func (f *DummyFile) Remote() string {
	return f.Name + " (remote)"
}

func (f *DummyFile) ModTime(ctx context.Context) time.Time {
	return time.Now()
}

func (f *DummyFile) Size() int64 {
	return int64(len(f.Name))
}

func (f *DummyFile) Fs() fs.Info {
	return &DummyFs{}
}

func (f *DummyFile) Hash(ctx context.Context, ty hash.Type) (string, error) {
	return "244aa7266b3f5a08321b403b2c59baeba5539b19", nil
}

func (f *DummyFile) Storable() bool {
	return true
}

// List the objects and directories in dir into entries.  The
// entries can be returned in any order but should be for a
// complete directory.
//
// dir should be "" to list the root, and should not have
// trailing slashes.
//
// This should return ErrDirNotFound if the directory isn't
// found.
func (f *DummyFs) List(ctx context.Context, dir string) (entries fs.DirEntries, err error) {
	log.Println("List")
	entries = append(entries,
		&DummyFile{Name: "dummy file 1"}, // not yet an "Object" or "Directory"
		&DummyFile{Name: "dummy file 2"},
	)
	return entries, nil
}

// NewObject finds the Object at remote.  If it can't be found
// it returns the error ErrorObjectNotFound.
//
// If remote points to a directory then it should return
// ErrorIsDir if possible without doing any extra work,
// otherwise ErrorObjectNotFound.
func (f *DummyFs) NewObject(ctx context.Context, remote string) (fs.Object, error) {
	log.Println("NewObject")
	return nil, nil
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
func (f *DummyFs) Put(ctx context.Context, in io.Reader, src fs.ObjectInfo, options ...fs.OpenOption) (fs.Object, error) {
	log.Println("Put")
	return nil, nil
}

// Mkdir makes the directory (container, bucket)
//
// Shouldn't return an error if it already exists
func (f *DummyFs) Mkdir(ctx context.Context, dir string) error {
	log.Println("Mkdir")
	return nil
}

// Rmdir removes the directory (container, bucket) if empty
//
// Return an error if it doesn't exist or isn't empty
func (f *DummyFs) Rmdir(ctx context.Context, dir string) error {
	log.Println("Rmdir")
	return nil
}

// Check the interfaces are satisfied
var (
	_ fs.Fs = &DummyFs{}
	// _ fs.Copier      = &Fs{}
	// _ fs.PutStreamer = &Fs{}
	// _ fs.ListRer     = &Fs{}
	// _ fs.Object      = &Object{}
	// _ fs.MimeTyper   = &Object{}
)
