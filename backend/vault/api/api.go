package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/antchfx/htmlquery"
	"github.com/rclone/rclone/backend/vault/cache"
	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/fs/fshttp"
	"github.com/rclone/rclone/lib/rest"
)

const (
	// maxResponseBody limit in bytes when reading a response body.
	maxResponseBody    = 1 << 24
	VaultVersionHeader = "X-VAULT-API-VERSION"
)

var (
	ErrUserNotFound   = errors.New("user not found")
	ErrAmbiguousQuery = errors.New("ambiguous query")
)

type Api struct {
	Endpoint         string
	Username         string
	Password         string
	VersionSupported string

	client    *rest.Client
	loginPath string
	timeout   time.Duration
	cache     *cache.Cache
}

func New(endpoint, username, password string) *Api {
	ctx := context.Background()
	return &Api{
		Endpoint:         endpoint,
		Username:         username,
		Password:         password,
		VersionSupported: "1",
		client:           rest.NewClient(fshttp.NewClient(ctx)).SetRoot(endpoint),
		loginPath:        "/accounts/login/", // trailing slash required, cf. django APPEND_SLASH
		timeout:          5 * time.Second,
		cache:            cache.New(),
	}
}

// Version returns the API version transmitted in HTTP (proposed).
func (api *Api) Version() string {
	opts := rest.Opts{
		Method: "GET",
		Path:   "/",
	}
	resp, err := api.client.Call(context.TODO(), &opts)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	return resp.Header.Get(VaultVersionHeader)
}

func (api *Api) String() string {
	return fmt.Sprintf("vault api (v%s)", api.VersionSupported)
}

// Login sets up a session, which should be valid for the client until logout
// (or timeout).
func (api *Api) Login() (err error) {
	var u *url.URL
	if u, err = url.Parse(api.Endpoint); err != nil {
		return err
	}
	u.Path = api.loginPath
	loginURL := u.String()
	resp, err := http.Get(loginURL)
	if err != nil {
		return fmt.Errorf("cannot access login url: %w", err)
	}
	defer resp.Body.Close()
	// Parse out the CSRF token: <input type="hidden"
	// name="csrfmiddlewaretoken"
	// value="CCBQ9qqG3ylgR1MaYBc6UCw4tlxR7rhP2Qs4uvIMAf1h7Dd4xtv5azTQJRgJ1y2I">
	//
	// TODO: move to a token based auth for the API:
	// https://stackoverflow.com/q/21317899/89391
	doc, err := htmlquery.Parse(resp.Body)
	if err != nil {
		return fmt.Errorf("html: %w", err)
	}
	token := htmlquery.SelectAttr(
		htmlquery.FindOne(doc, `//input[@name="csrfmiddlewaretoken"]`),
		"value",
	)
	jar, err := cookiejar.New(nil)
	if err != nil {
		return err
	}
	// Need to reparse, api may live on a different path.
	u, err = url.Parse(api.Endpoint)
	if err != nil {
		return err
	}
	jar.SetCookies(u, []*http.Cookie{&http.Cookie{
		Name:  "csrftoken",
		Value: token,
	}})
	client := http.Client{
		Jar:     jar,
		Timeout: api.timeout,
	}
	// We could use PostForm, but we need to set extra headers.
	data := url.Values{}
	data.Set("username", api.Username)
	data.Set("password", api.Password)
	data.Set("csrfmiddlewaretoken", token)
	req, err := http.NewRequest("POST", loginURL, strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// You are seeing this message because this HTTPS site requires a “Referer
	// header” to be sent by your Web browser, but none was sent. This header
	// is required for security reasons, to ensure that your browser is not
	// being hijacked by third parties.
	req.Header.Set("Referer", loginURL)
	resp, err = client.Do(req)
	if err != nil {
		return fmt.Errorf("vault login post: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := ioutil.ReadAll(resp.Body)
		log.Println(string(b))
		return fmt.Errorf("login failed with: %v", resp.StatusCode)
	}
	api.client.SetCookie(jar.Cookies(u)...)
	return nil
}

// Logout drops the session.
func (api *Api) Logout() {
	api.client.SetHeader("Cookie", "")
}

// Call exposes the current client for call that are not mapped to separate
// functions. TODO: we should not leak this.
func (api *Api) Call(ctx context.Context, opts *rest.Opts) (*http.Response, error) {
	return api.client.Call(ctx, opts)
}

func (api *Api) CallJSON(ctx context.Context, opts *rest.Opts, req, resp interface{}) (*http.Response, error) {
	return api.client.CallJSON(ctx, opts, req, resp)
}

// SplitPath returns the treenodes for the collection and leaf object for a
// given path as well as the path without the collection. It is an error if the
// collection cannot be found.
func (api *Api) SplitPath(p string) (*PathInfo, error) {
	if !strings.HasPrefix(p, "/") {
		return nil, fmt.Errorf("absolute path required: %v", p)
	}
	var (
		err   error
		pi    = PathInfo{}
		parts = strings.Split(p, "/")
	)
	switch {
	case len(parts) < 2:
		return nil, fmt.Errorf("invalid path")
	default:
		pi.CollectionTreeNode, err = api.ResolvePath("/" + parts[0])
		if err != nil {
			return nil, err
		}
		pi.LeafTreeNode, err = api.ResolvePath(p)
		if err != nil {
			return nil, err
		}
		pi.RelativePath = strings.Join(parts[2:], "/")
		if pi.RelativePath == "" {
			pi.RelativePath = "/"
		}
	}
	return &pi, nil
}

// DepositStatus returns information about a specific deposit.
func (api *Api) DepositStatus(id int64) (*DepositStatus, error) {
	opts := rest.Opts{
		Method: "GET",
		Path:   "deposit_status",
		Parameters: url.Values{
			"deposit_id": []string{fmt.Sprintf("%d", id)},
		},
	}
	var ds DepositStatus
	resp, err := api.client.CallJSON(context.TODO(), &opts, nil, &ds)
	if err != nil {
		log.Println("xxx")
		return nil, err
	}
	defer resp.Body.Close()
	return &ds, nil
}

// Create a collection with a given name. This would corresponds to a directory
// in the root of a mount.
func (api *Api) CreateCollection(name string) error {
	opts := rest.Opts{
		Method:      "POST",
		Path:        "/collections/",
		Body:        strings.NewReader(fmt.Sprintf(`{"name": %q}`, name)),
		ContentType: "application/json",
		ExtraHeaders: map[string]string{
			"X-CSRFTOKEN": api.csrfToken(),
			"Referer":     api.refererURL("collections"),
		},
	}
	resp, err := api.client.CallJSON(context.TODO(), &opts, nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// CreateFolder creates a folder below a given parent treenode.
func (api *Api) CreateFolder(parent *TreeNode, name string) error {
	fs.Debugf(api, "creating folder %v with parent %v", name, parent.Id)
	parentURL := fmt.Sprintf("%s/treenodes/%d/", api.Endpoint, parent.Id)
	opts := rest.Opts{
		Method: "POST",
		Path:   "/treenodes/",
		Body: strings.NewReader(fmt.Sprintf(`{
			"name": %q,
		    "node_type": "FOLDER",
		    "parent": %q
		}`, name, parentURL)),
		ContentType: "application/json",
		ExtraHeaders: map[string]string{
			"X-CSRFTOKEN": api.csrfToken(),
			"Referer":     api.refererURL("treenodes"),
		},
	}
	resp, err := api.client.CallJSON(context.TODO(), &opts, nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// List returns the immediate children of a treenode.
func (api *Api) List(t *TreeNode) ([]*TreeNode, error) {
	if t == nil {
		return nil, nil
	}
	return api.FindTreeNodes(url.Values{
		"parent": []string{fmt.Sprintf("%d", t.Id)},
	})
}

// ResolvePath resolve an absolute path to a treenode object.
func (api *Api) ResolvePath(p string) (*TreeNode, error) {
	t, err := api.root()
	if err != nil {
		return nil, err
	}
	// This method only resolves absolute paths. TODO: Should we rather fail?
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	// segments: /a/b/c -> [a b c], /a/b/ -> [a b]
	segments := strings.Split(strings.TrimRight(p, "/"), "/")[1:]
	if p != "" && p != "/" && len(segments) == 0 {
		return nil, fs.ErrorObjectNotFound
	}
	for len(segments) > 0 {
		ts, err := api.FindTreeNodes(url.Values{
			"parent": []string{strconv.Itoa(int(t.Id))},
			"name":   []string{segments[0]},
		})
		switch {
		case err != nil:
			return nil, err
		case len(ts) == 0:
			return nil, fs.ErrorObjectNotFound
		case len(ts) > 1:
			log.Printf("[skip] invalid path, more than one parent node found for %v: %v (%v)", t.Id, t.Name, t.NodeType)
			continue
		}
		t, segments = ts[0], segments[1:]
	}
	return t, nil
}

func (api *Api) RegisterDeposit(root string, rdr *RegisterDepositRequest) (id int64, err error) {
	opts := rest.Opts{
		Method: "POST",
		Path:   "/register_deposit",
	}
	var depositResp RegisterDepositResponse
	resp, err := api.client.CallJSON(context.Background(), &opts, rdr, &depositResp)
	if err != nil {
		// TODO: we need warning deposit here to check whether files already
		// exist; do some kind of "--force" by default
		return 0, fmt.Errorf("api failed: %v", err)
	}
	defer resp.Body.Close()
	fs.Logf(api, "deposit registered: %v", depositResp.ID)
	return depositResp.ID, nil
}

// TreeNodeToCollection turns a treenode to a collection.
func (api *Api) TreeNodeToCollection(t *TreeNode) (*Collection, error) {
	result, err := api.FindCollections(url.Values{
		"tree_node": []string{fmt.Sprintf("%d", t.Id)},
	})
	if err != nil {
		return nil, err
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("collection not found")
	}
	return result[0], nil
}

func (api *Api) refererURL(suffix string) string {
	return fmt.Sprintf("%s/%s", strings.TrimRight(api.Endpoint, "/"), suffix)
}

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
	resp, err := api.client.Call(ctx, &opts)
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

// root returns the organization not for the api user.
func (api *Api) root() (*TreeNode, error) {
	organization, err := api.Organization()
	if err != nil {
		return nil, err
	}
	return api.GetTreeNode(organization.TreeNodeIdentifier())
}

// User returns the current user.
func (api *Api) User() (*User, error) {
	userList, err := api.FindUsers(url.Values{
		"username": []string{api.Username},
	})
	switch {
	case err != nil:
		return nil, err
	case len(userList) == 0:
		return nil, ErrUserNotFound
	case len(userList) > 1:
		return nil, ErrAmbiguousQuery
	}
	return userList[0], nil
}

// Organization returns the Organization of the current user.
func (api *Api) Organization() (*Organization, error) {
	u, err := api.User()
	if err != nil {
		return nil, err
	}
	return api.GetOrganization(u.OrganizationIdentifier())
}
