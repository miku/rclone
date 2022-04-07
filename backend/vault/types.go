package vault

// TODO: we may need to get all results for a given query.
// TODO: add pagination later
// TODO: add a few convenience methods for the api
// TODO: could use a "dircache" locally to speed up operations (with some low
// ttl, like 60s)
import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/antchfx/htmlquery"
	"github.com/rclone/rclone/lib/rest"
)

var (
	ErrPathNotFound = errors.New("path not found")
	ErrInvalidPath  = errors.New("invalid path")
)

// Api handles all interactions with the vault API. TODO: handle auth for API.
type Api struct {
	endpoint string       // e.g. http://127.0.0.1:8000/api
	username string       // vault username, required for various operations
	password string       // vault password
	srv      *rest.Client // the connection to the vault server
}

// ApiError for allows to transmit HTTP code and message.
type ApiError struct {
	StatusCode int
	Message    string
}

func (e *ApiError) Error() string {
	if e.StatusCode != 0 {
		return fmt.Sprintf("api error: HTTP %d: %s", e.StatusCode, e.Message)
	} else {
		return fmt.Sprintf("api error: %s", e.Message)
	}
}

// Login uses the django login page to obtain a session id.
func (api *Api) Login() error {
	// http://127.0.0.1:8000/accounts/login/
	loginURL := strings.Replace(api.endpoint, "/api", "/accounts/login/", 1)
	resp, err := http.Get(loginURL)
	if err != nil {
		return fmt.Errorf("cannot access login url: %w")
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
	u, err := url.Parse(loginURL)
	if err != nil {
		return err
	}
	jar.SetCookies(u, []*http.Cookie{&http.Cookie{
		Name:  "csrftoken",
		Value: token,
	}})
	client := http.Client{
		Jar:     jar,
		Timeout: 5 * time.Second,
	}
	vs := url.Values{}
	vs.Set("username", api.username)
	vs.Set("password", api.password)
	vs.Set("csrfmiddlewaretoken", token)
	resp, err = client.PostForm(loginURL, vs)
	if err != nil {
		return fmt.Errorf("post: %w", err)
	}
	defer resp.Body.Close()
	for _, c := range jar.Cookies(u) {
		api.srv.SetCookie(c)
	}
	return nil
}

// root returns the root treenode for the api user, that is their
// organization's treenode.
func (api *Api) root() (*TreeNode, error) {
	userList, err := api.FindUsers(url.Values{
		"username": []string{api.username},
	})
	switch {
	case err != nil:
		return nil, err
	case len(userList) == 0:
		return nil, &ApiError{Message: "no user found"}
	case len(userList) > 1:
		return nil, &ApiError{Message: "ambiguous user query"}
	}
	u := userList[0]
	if u.Organization == "" {
		return nil, &ApiError{Message: "user does not belong to an organization"}
	}
	organization, err := api.GetOrganization(u.OrganizationIdentifier())
	if err != nil {
		return nil, err
	}
	return api.GetTreeNode(organization.TreeNodeIdentifier())
}

// resolvePath takes a path and turns it into a TreeNode representing that
// object (org, collection, folder, file). A path is case sensitive.
func (api *Api) resolvePath(path string) (*TreeNode, error) {
	t, err := api.root()
	log.Println(err)
	if err != nil {
		return nil, err
	}
	// segments: /a/b/c -> [a b c], /a/b/ -> [a b]
	segments := strings.Split(strings.TrimRight(path, "/"), "/")[1:]
	if path != "" && path != "/" && len(segments) == 0 {
		return nil, ErrPathNotFound
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
			return nil, ErrPathNotFound
		case len(ts) > 1:
			return nil, ErrInvalidPath
		}
		t, segments = ts[0], segments[1:]
	}
	return t, nil
}

func (api *Api) CreateCollection(name string) error {
	var (
		link     = fmt.Sprintf("%s/collections", api.endpoint)
		req, err = http.NewRequest("POST", link, strings.NewReader(fmt.Sprintf(`{"name": %v"`, name)))
	)
	req.Header.Add("Content-Type", "application/json")
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("api failed with %v", resp.StatusCode)
	}
	io.Copy(os.Stderr, resp.Body)
	return nil
}

// List list all children under a given treenode.
func (api *Api) List(t *TreeNode) (result []*TreeNode, err error) {
	if t == nil {
		return
	}
	return api.FindTreeNodes(url.Values{
		"parent": []string{fmt.Sprintf("%d", t.Id)},
	})
}

// FindUsers finds users, filtered by query parameters.
func (api *Api) FindUsers(vs url.Values) (result []*User, err error) {
	// ...?last_login=&last_login__gt=&last_login__gte=&last_login__lt=&last_lo
	// gin__lte=&username__contains=a&username__endswith=&username=&username__icontains
	// =&username__iexact=&username__startswith=
	// 	&first_name__contains=&first_name__endswith=&first_name=&first_name__
	// icontains=&first_name__iexact=&first_name__startswith=&last_name__contains=&last
	// _name__endswith=&last_name=&last_name__icontains=&last_name__iexact=&last_name__
	// startswith=&date_joined=&date_joined__gt=&date_joined__gte=&date_joined__lt=&dat
	// e_joined__lte=&organization=
	// var (
	// 	link       = fmt.Sprintf("%s/users/?%s", api.endpoint, vs.Encode())
	// 	resp, herr = http.Get(link) // move to pester or other retry library
	// )
	// if herr != nil {
	// 	return nil, herr
	// }
	opts := &rest.Opts{
		Method:     "GET",
		Path:       "/users/",
		Parameters: vs,
	}
	resp, err := api.srv.Call(context.Background(), opts)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, &ApiError{StatusCode: resp.StatusCode, Message: "users"}
	}
	var (
		dec  = json.NewDecoder(resp.Body)
		list UserList
	)
	if err := dec.Decode(&list); err != nil {
		return nil, err
	}
	for _, v := range list.Results {
		result = append(result, v)
	}
	return result, nil
}

// GetOrganization returns a single organization by id. If id look like a URL,
// use it directly.
func (api *Api) GetOrganization(id string) (*Organization, error) {
	opts := &rest.Opts{
		Method: "GET",
		Path:   fmt.Sprintf("/organizations/%v", id),
	}
	resp, err := api.srv.Call(context.Background(), opts)
	// XX: resp, err := http.Get(link) // move to pester or other retry library
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, &ApiError{StatusCode: resp.StatusCode, Message: "organization"}
	}
	var (
		dec = json.NewDecoder(resp.Body)
		doc Organization
	)
	if err := dec.Decode(&doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

// FindOrganizations finds organizations, filtered by query parameters.
func (api *Api) FindOrganizations(vs url.Values) (result []*Organization, err error) {
	// XX: link := fmt.Sprintf("%s/organizations/?%s", api.endpoint, vs.Encode())
	// XX: resp, err := http.Get(link) // move to pester or other retry library
	opts := &rest.Opts{
		Method:     "GET",
		Path:       "/organizations/",
		Parameters: vs,
	}
	resp, err := api.srv.Call(context.Background(), opts)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, &ApiError{StatusCode: resp.StatusCode, Message: "organizations"}
	}
	var (
		dec  = json.NewDecoder(resp.Body)
		list OrganizationList
	)
	if err := dec.Decode(&list); err != nil {
		return nil, err
	}
	for _, v := range list.Results {
		result = append(result, v)
	}
	return result, nil
}

// GetTreeNode returns a single treenode by id.
func (api *Api) GetTreeNode(id string) (*TreeNode, error) {
	// var link string
	// if strings.HasPrefix(id, "http") {
	// 	link = id
	// } else {
	// 	link = fmt.Sprintf("%s/treenodes/%s", api.endpoint, id)
	// }
	// resp, err := http.Get(link) // move to pester or other retry library
	opts := &rest.Opts{
		Method: "GET",
		Path:   fmt.Sprintf("/treenodes/%v", id),
	}
	resp, err := api.srv.Call(context.Background(), opts)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, &ApiError{StatusCode: resp.StatusCode, Message: "treenode"}
	}
	var (
		dec = json.NewDecoder(resp.Body)
		doc TreeNode
	)
	if err := dec.Decode(&doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

// FindTreeNodes finds treenodes, filtered by query parameters.
func (api *Api) FindTreeNodes(vs url.Values) (result []*TreeNode, err error) {
	// ?id=1&id__gt=&id__gte=&id__lt=&id__lte=&node_type__contains=&node_type__
	// endswith=&node_type=&node_type__icontains=&node_type__iexact=&node_type__startsw
	// ith=&path__contains=&path__endswith=&path=&path__icontains=&path__iexact=&path__
	// startswith=&name__contains=&name__endswith=&name=&name__icontains=&name__iexact=
	// &name__startswith=&md5_sum__contains=&md5_sum__endswith=&md5_sum=&md5_sum__icont
	// ains=&md5_sum__iexact=&md5_sum__startswith=&sha1_sum__contains=&sha1_sum__endswi
	// th=&sha1_sum=&sha1_sum__icontains=&sha1_sum__iexact=&sha1_sum__startswith=&sha25
	// 6_sum__contains=&sha256_sum__endswith=&sha256_sum=&sha256_sum__icontains=&sha256
	// _sum__iexact=&sha256_sum__startswith=&size=&size__gt=&size__gte=&size__lt=&size_
	// _lte=&file_type__contains=&file_type__endswith=&file_type=&file_type__icontains=
	// &file_type__iexact=&file_type__startswith=&uploaded_at=&uploaded_at__gt=&uploade
	// d_at__gte=&uploaded_at__lt=&uploaded_at__lte=&pre_deposit_modified_at=&pre_depos
	// it_modified_at__gt=&pre_deposit_modified_at__gte=&pre_deposit_modified_at__lt=&p
	// re_deposit_modified_at__lte=&modified_at=&modified_at__gt=&modified_at__gte=&mod
	// ified_at__lt=&modified_at__lte=&uploaded_by=&comment__contains=&comment__endswit
	// h=&comment=&comment__icontains=&comment__iexact=&comment__startswith=&parent=
	// XX: link := fmt.Sprintf("%s/treenodes/?%s", api.endpoint, vs.Encode())
	// log.Println(link)
	// TODO: api.srv.Call(context.Background(), &rest.Opts{})
	resp, err := api.srv.Call(context.Background(), &rest.Opts{
		Method:     "GET",
		Path:       "/treenodes/",
		Parameters: vs,
	})

	// XX: resp, err := http.Get(link) // move to srv
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, &ApiError{StatusCode: resp.StatusCode, Message: "treenodes"}
	}
	var (
		dec  = json.NewDecoder(resp.Body)
		list TreeNodeList
	)
	if err := dec.Decode(&list); err != nil {
		return nil, err
	}
	for _, v := range list.Results {
		result = append(result, v)
	}
	// Any more results?
	return result, nil
}

// User represents a single user.
type User struct {
	DateJoined   string `json:"date_joined"`
	FirstName    string `json:"first_name"`
	IsActive     bool   `json:"is_active"`
	IsStaff      bool   `json:"is_staff"`
	IsSuperuser  bool   `json:"is_superuser"`
	LastLogin    string `json:"last_login"`
	LastName     string `json:"last_name"`
	Organization string `json:"organization"`
	Url          string `json:"url"`
	Username     string `json:"username"`
}

func (u *User) OrganizationIdentifier() string {
	if u.Organization == "" {
		return u.Organization
	}
	if !strings.HasPrefix(u.Organization, "http") {
		return u.Organization
	}
	parts := strings.Split(strings.TrimRight(u.Organization, "/"), "/")
	if len(parts) == 0 {
		return ""
	}
	candidate := parts[len(parts)-1]
	if _, err := strconv.Atoi(candidate); err != nil {
		return ""
	}
	return candidate
}

// UserList from API, via JSONGen.
type UserList struct {
	Count    int64       `json:"count"`
	Next     interface{} `json:"next"`
	Previous interface{} `json:"previous"`
	Results  []*User     `json:"results"`
}

// Organization represents a single document.
type Organization struct {
	Name       string `json:"name"`
	Plan       string `json:"plan"`
	QuotaBytes int64  `json:"quota_bytes"`
	TreeNode   string `json:"tree_node"`
	Url        string `json:"url"`
}

func (o *Organization) TreeNodeIdentifier() string {
	if o.TreeNode == "" {
		return o.TreeNode
	}
	if !strings.HasPrefix(o.TreeNode, "http") {
		return o.TreeNode
	}
	parts := strings.Split(strings.TrimRight(o.TreeNode, "/"), "/")
	if len(parts) == 0 {
		return ""
	}
	candidate := parts[len(parts)-1]
	if _, err := strconv.Atoi(candidate); err != nil {
		return ""
	}
	return candidate
}

// OrganizationList contains a list of organizations, e.g. from search.
type OrganizationList struct {
	Count    int64           `json:"count"`
	Next     interface{}     `json:"next"`
	Previous interface{}     `json:"previous"`
	Results  []*Organization `json:"results"`
}

// TreeNode is a single document.
type TreeNode struct {
	Comment              interface{} `json:"comment"`
	ContentUrl           interface{} `json:"content_url"`
	FileType             interface{} `json:"file_type"`
	Id                   int64       `json:"id"`
	Md5Sum               interface{} `json:"md5_sum"`
	ModifiedAt           string      `json:"modified_at"`
	Name                 string      `json:"name"`
	NodeType             string      `json:"node_type"`
	Parent               interface{} `json:"parent"`
	Path                 string      `json:"path"`
	PreDepositModifiedAt string      `json:"pre_deposit_modified_at"`
	Sha1Sum              interface{} `json:"sha1_sum"`
	Sha256Sum            interface{} `json:"sha256_sum"`
	Size                 interface{} `json:"size"`
	UploadedAt           string      `json:"uploaded_at"`
	UploadedBy           interface{} `json:"uploaded_by"`
	Url                  string      `json:"url"`
}

// TreeNodeList for a list of treenodes.
type TreeNodeList struct {
	Count    int64       `json:"count"`
	Next     interface{} `json:"next"`
	Previous interface{} `json:"previous"`
	Results  []*TreeNode `json:"results"`
}
