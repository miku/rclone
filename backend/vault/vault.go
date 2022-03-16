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
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/fs/config/configmap"
	"github.com/rclone/rclone/fs/hash"
)

func init() {
	fs.Register(&fs.RegInfo{
		Name:        "vault",
		Description: "Internet Archive Vault Digital Preservation System",
		Prefix:      "vault",
		NewFs:       NewFs,
		Options: []fs.Option{
			{
				Name:    "username",
				Help:    "vault username (needs to belong to an organization to be usable)",
				Default: 0,
			},
			{
				Name:    "collection",
				Help:    "identifier of the collection for data transfer",
				Default: 0,
			},
			{
				Name:    "url",
				Help:    "base URL of vault service",
				Default: "http://localhost:8000",
			},
		},
	})
}

func NewFs(ctx context.Context, name, root string, cm configmap.Mapper) (fs.Fs, error) {
	// The name and root are omitted, as there is only one Internet Archive
	// with a single namespace.
	organization, ok := cm.Get("organization")
	if !ok {
		return nil, fmt.Errorf("missing organization id")
	}
	return &Fs{
		name:        name,
		root:        root,
		baseURL:     "http://localhost:8000",
		description: "Internet Archive Vault Digital Preservation System",
	}, nil
}

// Fs represents Internet Archive collections and items.
type Fs struct {
	description string
	name        string
	root        string
	username    string // user name (may only need this)
	baseURL     string // e.g. http://localhost:8000
}

// Name of the remote (as passed into NewFs)
func (f *Fs) Name() string {
	return f.name
}

// Root of the remote (as passed into NewFs)
func (f *Fs) Root() string {
	return f.root
}

// String returns a description of the FS
func (f *Fs) String() string {
	return f.description
}

// Precision of the ModTimes in this Fs.
func (f *Fs) Precision() time.Duration {
	return 1 * time.Second
}

// Returns the supported hash types of the filesystem
func (f *Fs) Hashes() hash.Set {
	return hash.Set(hash.MD5 | hash.SHA1 | hash.SHA256)
}

// Features returns the optional features of this Fs.
func (f *Fs) Features() *fs.Features {
	return &fs.Features{
		CaseInsensitive:         true,
		DuplicateFiles:          false,
		CanHaveEmptyDirectories: true,
		BucketBased:             true,
		BucketBasedRootOK:       true,
		SetTier:                 false,
		GetTier:                 false,
		ServerSideAcrossConfigs: true,
		IsLocal:                 false,
		SlowModTime:             true,
		SlowHash:                true,
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

	log.Printf("List: %s", dir)

	link := fmt.Sprintf("%s/api/collections/?organization=%s", f.baseURL, f.organization)
	log.Println(link)
	resp, err := http.Get(link)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("failed to list directory: %s", dir)
	}
	var payload struct {
		Collections []struct {
			Id   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"collections"`
	}
	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(&payload); err != nil {
		return nil, err
	}
	for _, c := range payload.Collections {
		entries = append(entries, &DummyFile{Name: c.Name})
	}
	// 	entries = append(entries,
	// 		&DummyFile{Name: "dummy file 1"}, // not yet an "Object" or "Directory"
	// 		&DummyFile{Name: "dummy file 2"},
	// 	)
	return entries, nil
}

func (f *Fs) list(ctx context.Context, dir string) (entries fs.DirEntries, err error) {
	// [ ] org for username; http://127.0.0.1:8000/api/users/?last_login=&last_login__gt=&last_login__gte=&last_login__lt=&last_login__lte=&username__contains=&username__endswith=&username=admin&username__icontains=&username__iexact=&username__startswith=&first_name__contains=&first_name__endswith=&first_name=&first_name__icontains=&first_name__iexact=&first_name__startswith=&last_name__contains=&last_name__endswith=&last_name=&last_name__icontains=&last_name__iexact=&last_name__startswith=&date_joined=&date_joined__gt=&date_joined__gte=&date_joined__lt=&date_joined__lte=&organization=
	// [ ] treenode for org

	return nil, nil
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
