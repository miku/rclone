package api

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/cookiejar"
	"net/http/httputil"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/antchfx/htmlquery"
	"github.com/rclone/rclone/fs/fshttp"
	"github.com/rclone/rclone/lib/rest"
)

// maxResponseBody limit in bytes when reading a response body.
const maxResponseBody = 1 << 24

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

// Login sets up a session, which should be valid for the client until logout
// (or timeout).
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
	data := url.Values{}
	data.Set("username", api.Username)
	data.Set("password", api.Password)
	data.Set("csrfmiddlewaretoken", token)
	req, err := http.NewRequest("POST", loginURL, strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencode")
	req.Header.Set("Referer", loginURL)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("login: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, err := httputil.DumpResponse(resp, true)
		if err != nil {
			return err
		}
		return fmt.Errorf("login: %v", string(b))
	}
	api.srv.SetCookie(jar.Cookies(u))
	return nil
}

// Logout drops the session.
func (api *Api) Logout() error {
	api.srv.SetHeader("Cookie", "")

}
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

// csrfToken retrieves a CSRF token. Returns an empty string on failure.
func (api *Api) csrfToken() string {
	ctx := context.Background()
	opts := rest.Opts{
		Method: "GET",
		Path:   "/users/", // any path valid path should do
		ExtraHeaders: map[string]string{
			"Accept": "text/html",
		},
	}
	resp, err := api.srv.Call(ctx, &opts)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return ""
	}
	b, err := ioutil.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	if err != nil {
		return ""
	}
	re := regexp.MustCompile(`csrfToken:[ ]*"([^"]*)"`)
	matches := re.FindStringSubmatch(string(b))
	if len(matches) == 2 {
		return matches[1]
	}
	return ""
}

func (api *Api) root() (*TreeNode, error) {
	// TODO: return the root treenode, i.e. the treenode for the organization
	// the user belongs to
}
func (api *Api) resolvePath(path string) (*TreeNode, error) {
	// TODO: resolve a path to a treenode
}
