package api

import (
	"context"
	"fmt"
	"net/url"

	"github.com/rclone/rclone/lib/rest"
)

// Organization represents a single document.
type Organization struct {
	Name       string `json:"name"`
	Plan       string `json:"plan"`
	QuotaBytes int64  `json:"quota_bytes"`
	TreeNode   string `json:"tree_node"`
	URL        string `json:"url"`
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
	URL          string `json:"url"`
	Username     string `json:"username"`
}

// Collection payload.
type Collection struct {
	FixityFrequency    string `json:"fixity_frequency"`
	Name               string `json:"name"`
	Organization       string `json:"organization"`
	TargetGeolocations []struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	} `json:"target_geolocations"`
	TargetReplication int64  `json:"target_replication"`
	TreeNode          string `json:"tree_node"`
	URL               string `json:"url"` // http://127.0.0.1:8000/api/collections/1/
}

// TreeNode is node in the filesystem tree.
type TreeNode struct {
	Comment              interface{} `json:"comment"`
	ContentURL           interface{} `json:"content_url"`
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
	URL                  string      `json:"url"`
}

// Auxiliary structures
// --------------------

// File passed e.g. in deposit requests.
type File struct {
	FlowIdentifier       string `json:"flow_identifier"`
	Name                 string `json:"name"`
	PreDepositModifiedAt string `json:"pre_deposit_modified_at"` // e.g. 2018-04-13T08:06:48.000Z
	RelativePath         string `json:"relative_path"`
	Size                 int64  `json:"size"`
	Type                 string `json:"type"`
}

// FlowChunk is used to post data to the server, like FlowChunkGet or
// FlowChunkPost in vault.
type FlowChunk struct {
	DepositIdentifier string
	FileIdentifier    string
	Filename          string
	RelativePath      string
	Number            int
	Size              int64
	TotalSize         int64
	NumChunks         int
	TargetChunkSize   int64
}

// List objects
// ------------

type List struct {
	Count    int64       `json:"count"`
	Next     interface{} `json:"next"`
	Previous interface{} `json:"previous"`
}

// UserList from API, via JSONGen.
type UserList struct {
	List
	Results []*User `json:"results"`
}

// OrganizationList contains a list of organizations, e.g. from search.
type OrganizationList struct {
	List
	Results []*Organization `json:"results"`
}

// CollectionList contains a list of collections.
type CollectionList struct {
	List
	Results []*Collection `json:"results"`
}

// TreeNodeList for a list of treenodes.
type TreeNodeList struct {
	List
	Results []*TreeNode `json:"results"`
}

// Get methods
// -----------

func (api *Api) GetUser(id string) (*User, error) {
	var (
		opts = rest.Opts{
			Method: "GET",
			Path:   fmt.Sprintf("/users/%v/", id),
		}
		doc User
	)
	resp, err := api.Srv.CallJSON(context.TODO(), &opts, nil, &doc)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("api: users got %v", resp.StatusCode)
	}
	return &doc, nil
}

func (api *Api) GetOrganization(id string) (*Organization, error) {
	var (
		opts = rest.Opts{
			Method: "GET",
			Path:   fmt.Sprintf("/organizations/%v/", id),
		}
		doc Organization
	)
	resp, err := api.Srv.CallJSON(context.TODO(), &opts, nil, &doc)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("api: organizations got %v", resp.StatusCode)
	}
	return &doc, nil
}

func (api *Api) GetCollection(id string) (*Collection, error) {
	var (
		opts = rest.Opts{
			Method: "GET",
			Path:   fmt.Sprintf("/collections/%v/", id),
		}
		doc Collection
	)
	resp, err := api.Srv.CallJSON(context.TODO(), &opts, nil, &doc)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("api: collections got %v", resp.StatusCode)
	}
	return &doc, nil
}

func (api *Api) GetTreeNode(id string) (*TreeNode, error) {
	var (
		opts = rest.Opts{
			Method: "GET",
			Path:   fmt.Sprintf("/treenodes/%v/", id),
		}
		doc TreeNode
	)
	resp, err := api.Srv.CallJSON(context.TODO(), &opts, nil, &doc)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("api: treenodes got %v", resp.StatusCode)
	}
	return &doc, nil
}

// Find methods
// ------------

func (api *Api) FindUsers(vs url.Values) (result []*User, err error) {
	var (
		opts = rest.Opts{
			Method:     "GET",
			Path:       "/users/",
			Parameters: vs,
		}
		doc UserList
	)
	resp, err := api.Srv.CallJSON(context.TODO(), &opts, nil, &doc)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("api: users got %v", resp.StatusCode)
	}
	for i, v := range doc.Results {
		result = append(result, v)
	}
	return result, nil
}

func (api *Api) FindOrganizations(vs url.Values) (result []*Organization, err error) {
	var (
		opts = rest.Opts{
			Method:     "GET",
			Path:       "/organizations/",
			Parameters: vs,
		}
		doc OrganizationList
	)
	resp, err := api.Srv.CallJSON(context.TODO(), &opts, nil, &doc)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("api: organizations got %v", resp.StatusCode)
	}
	for i, v := range doc.Results {
		result = append(result, v)
	}
	return result, nil
}

func (api *Api) FindCollections(vs url.Values) (result []*Collection, err error) {
	var (
		opts = rest.Opts{
			Method:     "GET",
			Path:       "/collections/",
			Parameters: vs,
		}
		doc CollectionList
	)
	resp, err := api.Srv.CallJSON(context.TODO(), &opts, nil, &doc)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("api: collections got %v", resp.StatusCode)
	}
	for i, v := range doc.Results {
		result = append(result, v)
	}
	return result, nil
}

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
	var (
		opts = rest.Opts{
			Method:     "GET",
			Path:       "/treenodes/",
			Parameters: vs,
		}
		doc TreeNodeList
	)
	resp, err := api.Srv.CallJSON(context.TODO(), &opts, nil, &doc)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("api: treenodes got %v", resp.StatusCode)
	}
	for i, v := range doc.Results {
		result = append(result, v)
	}
	return result, nil
}
