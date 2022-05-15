package api

import (
	"net/url"

	"github.com/rclone/rclone/lib/rest"
)

type Api struct {
	Endpoint string
	Username string
	Password string
	srv      *rest.Client
}

// Login sets up a session and prepares the rest client.
func (api *Api) Login() error                                             {}
func (api *Api) Logout() error                                            {}
func (api *Api) CreateCollection(name string) error                       {}
func (api *Api) Exists(path string) (bool, error)                         {}
func (api *Api) List(path string)                                         {}
func (api *Api) FindCollections(vs url.Values) ([]*Collection, error)     {}
func (api *Api) FindOrganizations(vs url.Values) ([]*Organization, error) {}
func (api *Api) FindUsers(vs url.Values) ([]*User, error)                 {}
func (api *Api) FindTreeNodes(vs url.Values) ([]*TreeNode, error)         {}
func (api *Api) GetCollection(id int)                                     {}
func (api *Api) GetOrganization(id int)                                   {}
func (api *Api) GetUser(id int)                                           {}
func (api *Api) GetTreeNode(id int)                                       {}

func (api *Api) csrfToken() string {
	// TODO: obtain csrf token, required for various API operations
}
func (api *Api) root() (*TreeNode, error) {
	// TODO: return the root treenode, i.e. the treenode for the organization
	// the user belongs to
}
func (api *Api) resolvePath(path string) (*TreeNode, error) {
	// TODO: resolve a path to a treenode
}
