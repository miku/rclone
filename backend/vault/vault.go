package vault

import (
	"context"
	"io"
	"time"

	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/fs/config/configmap"
	"github.com/rclone/rclone/fs/config/configstruct"
	"github.com/rclone/rclone/fs/hash"
)

func init() {
	fs.Register(&fs.RegInfo{
		Name:        "vault",
		Description: "Internet Archive Vault Digital Preservation System",
		NewFs:       NewFs,
		Options:     []fs.Option{},
	})
}

func NewFs(ctx context.Context, name, root string, m configmap.Mapper) (fs.Fs, error) {
	opt := new(Options)
	err := configstruct.Set(m, opt)
	return &Fs{
		name: name,
		root: root,
		opt:  opt,
		features: &fs.Features{
			CaseInsensitive:         true,
			IsLocal:                 false,
			CanHaveEmptyDirectories: true,
			SlowHash:                true,
			BucketBased:             false,
		},
	}, nil
}

type Options struct {
	Username string `config:"username"`
	Password string `config:"password"`
	Endpoint string `config:"endpoing"`
}

type Fs struct {
	name     string
	root     string
	opt      Options
	features *fs.Features
}

// Fs Info
// -------

func (fs *Fs) Name() string             { return fs.name }
func (fs *Fs) Root() string             { return fs.root }
func (fs *Fs) String() string           { return fs.name }
func (fs *Fs) Precision() time.Duration { return 1 * time.Second }
func (fs *Fs) Hashes() hash.Set         { return hash.Set(hash.None) }
func (fs *Fs) Features() *fs.Features   { return fs.features }

// Fs Ops
// ------

func (fs *Fs) List(ctx context.Context, dir string) (fs.DirEntries, error) {
	// TODO: only need to list, if dir is a collection of folder
}
func (fs *Fs) NewObject(ctx context.Context, remote string) (fs.Object, error) {
	// TODO: only needs to return an object, if the remote exists and is a file
	// TODO: resolve remote to treenode and create an object
}
func (fs *Fs) Put(ctx context.Context, in io.Reader, src fs.ObjectInfo, options ...fs.OpenOptions) (fs.Object, error) {
	// TODO: src.Remote will be the "filename" and fs.root will be the dst
	// TODO: check if dst is actually a collection of folder
	// TODO: check if fs.root + "filename" already exists
	// TODO: start with a deposit per file approach first
	// TODO: register deposit for file, then use flow chunk endpoint to upload the data
	// TODO: at some point, we should have a treenode for the upload, return object
}
func (fs *Fs) Mkdir(ctx context.Context, dir string) error {
	// TODO: create collection or create folder
}
func (fs *Fs) Rmdir(ctx context.Context, dir string) error {
	// TODO: postpone
}

// Object
// ------

type Object struct {
	fs       fs.Fs
	remote   string
	treeNode *TreeNode
}

// Object DirEntry
// ---------------

func (o *Object) String()  { return o.remote }
func (o *Object) Remote()  { return o.remote }
func (o *Object) ModTime() {}
func (o *Object) Size()    {}

// Object Info
// -----------

func (o Object) Fs() fs.Info { return o.fs }
func (o Object) Hash()       {}
func (o Object) Storable()   {}

// Object Ops
// ----------

func (o Object) SetModTime() {}
func (o Object) Open()       {}
func (o Object) Update()     {}
func (o Object) Remove()     {}

// Dir
// ---

type Dir struct {
	remote   string
	treeNode *TreeNode
}

// Dir DirEntry
// ------------

func (dir *Dir) String()  { return o.remote }
func (dir *Dir) Remote()  { return o.remote }
func (dir *Dir) ModTime() {}
func (dir *Dir) Size()    {}

// Dir Ops

func (dir *Dir) Items() {}
func (dir *Dir) ID()    {}
