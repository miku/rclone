package icloud

import (
	"context"

	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/fs/config/configmap"
	"github.com/rclone/rclone/fs/config/configstruct"
)

func init() {
	fs.Register(&fs.RegInfo{
		Name:        "iCloud",
		Description: "iCloud Drive",
		NewFs:       NewFs,
		Options:     []fs.Option{},
	})
}

// Options for iCloud.
type Options struct {
	Username string
	Password string
}

func NewFs(ctx context.Context, name, root string, m configmap.Mapper) (fs.Fs, error) {
	var opt Options
	err := configstruct.Set(m, &opt)
	if err != nil {
		return nil, err
	}
	return &Fs{
		Name: name,
		Root: root,
	}, nil
}

type Fs struct {
	Name string
	Root string
}
