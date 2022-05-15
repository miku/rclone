package api

import "net/url"

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

// TreeNode is a single document.
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

func (api *Api) GetUser(id string) (*User, error)                 {}
func (api *Api) GetOrganization(id string) (*Organization, error) {}
func (api *Api) GetCollection(id string) (*Collection, error)     {}
func (api *Api) GetTreeNode(id string) (*TreeNode, error)         {}

// Find methods
// ------------

func (api *Api) FindUsers(vs url.Values) ([]*User, error)                 {}
func (api *Api) FindOrganizations(vs url.Values) ([]*Organization, error) {}
func (api *Api) FindCollections(vs url.Values) ([]*Collection, error)     {}
func (api *Api) FindTreeNodes(vs url.Values) ([]*TreeNode, error)         {}
