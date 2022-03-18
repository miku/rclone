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
package vault

import (
	"context"
	"fmt"
	"io"
	"log"
	"path/filepath"
	"strings"
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
	// log.Printf("NewFs: %s %s", name, root)
	opts := new(Options)
	err := configstruct.Set(cm, opts)
	if err != nil {
		return nil, err
	}
	return &Fs{
		name: name,
		root: root,
		opt:  *opts,
		api: &Api{
			Endpoint: opts.Endpoint,
			Username: opts.Username,
		},
	}, nil
}

// Fs represents Vault as a file system.
type Fs struct {
	name string
	root string
	opt  Options // parsed config options
	api  *Api
}

// Object represents a vault object, which is defined by a treeNode. Implements
// various interfaces.
type Object struct {
	fs       *Fs       // The Fs this object is part of
	treeNode *TreeNode // TreeNode object, which contains additional metadata, like modification time or hash
}

// Fs returns the Fs this object belongs to.
func (o *Object) Fs() fs.Info {
	return o.fs
}

// Hash return a selected checksum.
func (o *Object) Hash(_ context.Context, ty hash.Type) (string, error) {
	if o.treeNode == nil {
		return "", nil
	}
	switch ty {
	case hash.MD5:
		switch v := o.treeNode.Md5Sum.(type) {
		case string:
			return v, nil
		default:
			return "", nil
		}
	case hash.SHA1:
		switch v := o.treeNode.Sha1Sum.(type) {
		case string:
			return v, nil
		default:
			return "", nil
		}
	case hash.SHA256:
		switch v := o.treeNode.Sha256Sum.(type) {
		case string:
			return v, nil
		default:
			return "", nil
		}
	default:
		return "", nil
	}
}

func (o *Object) Storable() bool {
	// TODO: maybe only files and folders
	return true
}

// String returns the name of the object.
func (o *Object) String() string {
	if o == nil {
		return "<nil>"
	}
	return o.treeNode.Name
}

// Remote returns the remote path. This can fail silently. TODO: may cache this value.
//
// TODO: The remote name needs to be relative to the Fs.root, not the absolute path.
// TODO: Can use TreeNode path to access treenodes faster and to remote root from path.
func (o *Object) Remote() string {
	// log.Println("Remote")
	var (
		rootSegments []string = strings.Split(o.fs.root, "/")
		segments     []string
		err          error
	)
	var t *TreeNode = o.treeNode
OUTER:
	for {
		segments = append(segments, t.Name)
		switch t.Parent.(type) {
		case nil:
			break OUTER
		case string:
			s := t.Parent.(string)
			if s == "" {
				break OUTER
			}
			t, err = o.fs.api.GetTreeNode(s)
			if err != nil {
				return ""
			}
		default:
			log.Printf("warning: unknown parent type")
		}
	}
	for i, j := 0, len(segments)-1; i < j; i, j = i+1, j-1 {
		segments[i], segments[j] = segments[j], segments[i]
	}
	// Keep only segments which are not also in root.
	if len(rootSegments) > 0 && len(rootSegments[0]) == 0 {
		rootSegments = rootSegments[1:]
	}
	if len(segments) > 0 {
		segments = segments[1:]
	}
	log.Printf("%v -- %v", rootSegments, segments)
	var i int
	for i = range rootSegments {
		if i == len(segments) || rootSegments[i] != segments[i] {
			break
		}
	}
	segments = segments[i:]

	// remote top level directory, which is the name of the organization (TODO
	// maybe include it)
	remote := strings.Join(segments, "/")
	log.Printf("root: %v", o.fs.root)
	log.Printf("remote: %v", remote)
	// relpath := strings.Replace("/"+remote, o.fs.root+"/", "", 1)
	// log.Printf("relpath: %v", relpath)
	return remote
}

// ModTime returns the modification time of the object or epoch, if the time is not available.
func (o *Object) ModTime(_ context.Context) time.Time {
	epoch := time.Unix(0, 0)
	if o.treeNode == nil {
		return epoch
	}
	t, err := time.Parse("Jan 2, 2006 15:04:05 PST", o.treeNode.ModifiedAt)
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
func (f *Fs) List(ctx context.Context, dir string) (entries fs.DirEntries, err error) {
	//
	// dir      action
	// ---------------
	// ""       find ORG treenode (for user); then all item having that ORG as parent (mostly collections)
	// "/"      same as ""
	// "/a"     find collection named "a"
	// "/a/b"   find collection named "a", and find all parents
	// "/a/b/c" find collection named "a", parent "a" and name "b", parent "b" and "c"
	full := filepath.Join(f.root, dir)

	// log.Printf("List: %s", full)

	t, err := f.api.ResolvePath(full)
	if err != nil {
		return nil, err
	}
	// log.Printf("%v resolved to treenode %d: %v", full, t.Id, t)
	if t.NodeType == "FILE" {
		return nil, fmt.Errorf("path is a file")
	}

	tns, err := f.api.List(t)
	if err != nil {
		return nil, err
	}
	for _, tn := range tns {
		// log.Printf("%d\t%s\t%s", tn.Id, tn.NodeType, tn.Name)
		obj := &Object{
			fs:       f,
			treeNode: tn,
		}
		entries = append(entries, obj)
	}

	// link := fmt.Sprintf("%s/api/collections/?organization=%s", f.baseURL, f.organization)
	// log.Println(link)
	// resp, err := http.Get(link)
	// if err != nil {
	// 	return nil, err
	// }
	// defer resp.Body.Close()
	// if resp.StatusCode >= 400 {
	// 	return nil, fmt.Errorf("failed to list directory: %s", dir)
	// }
	// var payload struct {
	// 	Collections []struct {
	// 		Id   int64  `json:"id"`
	// 		Name string `json:"name"`
	// 	} `json:"collections"`
	// }
	// dec := json.NewDecoder(resp.Body)
	// if err := dec.Decode(&payload); err != nil {
	// 	return nil, err
	// }
	// for _, c := range payload.Collections {
	// 	entries = append(entries, &DummyFile{Name: c.Name})
	// }

	// entries = append(entries,
	// 	&DummyFile{Name: "dummy file 1"}, // not yet an "Object" or "Directory"
	// 	&DummyFile{Name: "dummy file 2"},
	// )
	return entries, nil
}

// Collection represents an internet archive collection. This is similar to a
// bucket or a directory, which can contain many collections or items.
type Collection struct{}

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
	return &Fs{}
}

func (f *DummyFile) Hash(ctx context.Context, ty hash.Type) (string, error) {
	return "244aa7266b3f5a08321b403b2c59baeba5539b19", nil
}

func (f *DummyFile) Storable() bool {
	return true
}

// NewObject finds the Object at remote.  If it can't be found
// it returns the error ErrorObjectNotFound.
//
// If remote points to a directory then it should return
// ErrorIsDir if possible without doing any extra work,
// otherwise ErrorObjectNotFound.
func (f *Fs) NewObject(ctx context.Context, remote string) (fs.Object, error) {
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
func (f *Fs) Put(ctx context.Context, in io.Reader, src fs.ObjectInfo, options ...fs.OpenOption) (fs.Object, error) {
	log.Println("Put")
	return nil, nil
}

// Mkdir makes the directory (container, bucket)
//
// Shouldn't return an error if it already exists
func (f *Fs) Mkdir(ctx context.Context, dir string) error {
	log.Println("Mkdir")
	return nil
}

// Rmdir removes the directory (container, bucket) if empty
//
// Return an error if it doesn't exist or isn't empty
func (f *Fs) Rmdir(ctx context.Context, dir string) error {
	log.Println("Rmdir")
	return nil
}

// Check the interfaces are satisfied
var (
	_ fs.Fs = &Fs{}
	// _ fs.Copier      = &Fs{}
	// _ fs.PutStreamer = &Fs{}
	// _ fs.ListRer     = &Fs{}
	// _ fs.Object      = &Object{}
	// _ fs.MimeTyper   = &Object{}
)
