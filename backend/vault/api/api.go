package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"time"

	"github.com/antchfx/htmlquery"
	"github.com/rclone/rclone/fs/fshttp"
	"github.com/rclone/rclone/lib/rest"
)

type Api struct {
	Endpoint  string
	Username  string
	Password  string
	srv       *rest.Client
	loginPath string
	timeout   time.Duration
}

func New(endpoint, username, password string) *Api {
	ctx := context.Background()
	return &Api{
		Endpoint:  endpoint,
		Username:  username,
		Password:  password,
		srv:       rest.NewClient(fshttp.NewClient(ctx)).SetRoot(endpoint),
		loginPath: "/accounts/login",
		timeout:   5 * time.Second,
	}
}

// Login sets up a session and prepares the rest client.
func (api *Api) Login() error {
	var u *url.URL
	if u, err := url.Parse(api.Endpoint); err != nil {
		return err
	}
	u.Path = loginPath
	loginURL := u.String()
	resp, err := http.Get(loginURL)
	if err != nil {
		return fmt.Errorf("login: failed at %v with %v", loginURL, err)
	}
	defer resp.Body.Close()
	doc, err := htmlquery.Parse(resp.Body)
	if err != nil {
		return fmt.Errorf("html: %v", err)
	}
	token := htmlquery.SelectAttr(
		htmlquery.FindOne(doc, `//input[@name="csrfmiddlewaretoken"]`),
		"value",
	)
	jar, err := cookiejar.New(nil)
	if err != nil {
		return fmt.Errorf("cookie: %v", err)
	}
	u, err = url.Parse(api.Endpoint)
	if err != nil {
		return err
	}
	jar.SetCookies(u, []*http.Cookie{
		{"csrftoken", token},
	})
	client := http.Client{
		Jar:     jar,
		Timeout: api.timeout,
	}
	// ...
}
func (api *Api) Logout() error                                            {}
func (api *Api) CreateCollection(name string) error                       {}
func (api *Api) Exists(path string) (bool, error)                         {}
func (api *Api) List(path string) ([]string, error)                       {}
func (api *Api) FindCollections(vs url.Values) ([]*Collection, error)     {}
func (api *Api) FindOrganizations(vs url.Values) ([]*Organization, error) {}
func (api *Api) FindUsers(vs url.Values) ([]*User, error)                 {}
func (api *Api) FindTreeNodes(vs url.Values) ([]*TreeNode, error)         {}
func (api *Api) GetCollection(id string)                                  {}
func (api *Api) GetOrganization(id string)                                {}
func (api *Api) GetUser(id string)                                        {}
func (api *Api) GetTreeNode(id string)                                    {}

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
