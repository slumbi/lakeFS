package gateway_test

import (
	"encoding/json"
	"github.com/ory/dockertest/v3"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/treeverse/lakefs/logging"

	"github.com/treeverse/lakefs/auth"
	"github.com/treeverse/lakefs/auth/model"
	"github.com/treeverse/lakefs/block"
	"github.com/treeverse/lakefs/gateway"
	"github.com/treeverse/lakefs/gateway/utils"
	"github.com/treeverse/lakefs/index"
	"github.com/treeverse/lakefs/testutil"
)

type playBackMockConf struct {
	ListenAddress   string `json:"listen_address"`
	BareDomain      string `json:"bare_domain"`
	AccessKeyId     string `json:"access_key_id"`
	AccessSecretKey string `json:"access_secret_Key"`
	CredentialType  string `json:"credential_type"`
	UserId          int    `json:"user_id"`
	Region          string `json:"Region"`
}

type dependencies struct {
	blocks block.Adapter
	auth   utils.GatewayAuthService
	meta   index.Index
}

func TestGatewayRecording(t *testing.T) {
	dirList, err := ioutil.ReadDir("testdata/recordings")
	if err != nil {
		t.Fatalf("Failed reading recording directories: %v", err)
	}
	for _, dir := range dirList {
		if !dir.IsDir() {
			continue
		}
		dirName := dir.Name()
		t.Run(dirName+" recording", func(t *testing.T) {
			setGlobalPlaybackParams(dirName)
			handler, _ := getBasicHandler(t, dirName)
			DoTestRun(handler, false, 1.0, t)
		})
	}
}

var (
	pool        *dockertest.Pool
	databaseUri string
)

func TestMain(m *testing.M) {
	var err error
	var closer func()
	pool, err = dockertest.NewPool("")
	if err != nil {
		log.Fatalf("Could not connect to Docker: %s", err)
	}
	databaseUri, closer = testutil.GetDBInstance(pool)
	code := m.Run()
	closer() // cleanup
	os.Exit(code)
}

type mockCollector struct{}

func (m *mockCollector) Collect(class, action string) {

}

var IdTranslator *testutil.UploadIdTranslator

func getBasicHandler(t *testing.T, testDir string) (http.Handler, *dependencies) {
	IdTranslator = &testutil.UploadIdTranslator{TransMap: make(map[string]string),
		ExpectedId: "",
		T:          t,
	}
	directory := filepath.Join("testdata", "recordings", testDir)

	mdb := testutil.GetDB(t, databaseUri, "lakefs_index")
	meta := index.NewDBIndex(mdb)

	blockAdapter := testutil.GetBlockAdapter(t, IdTranslator)

	authService := newGatewayAuth(t, directory)

	testutil.Must(t, meta.CreateRepo("example", "example-tzahi", "master"))
	server := gateway.NewServer(authService.Region,
		meta,
		blockAdapter,
		authService,
		authService.ListenAddress, authService.BareDomain, &mockCollector{})

	return server.Server.Handler, &dependencies{
		blocks: blockAdapter,
		auth:   authService,
		meta:   meta,
	}
}

func newGatewayAuth(t *testing.T, directory string) *playBackMockConf {
	m := new(playBackMockConf)
	fName := filepath.Join(directory, "simulation_config.json")
	confStr, err := ioutil.ReadFile(fName)
	if err != nil {
		t.Fatal(fName + " not found\n")
	}
	err = json.Unmarshal(confStr, m)
	if err != nil {
		t.Fatal("Failed to unmarshal configuration\n ")
	}
	return m
}

func (m *playBackMockConf) GetAPICredentials(accessKey string) (*model.Credential, error) {
	if accessKey != m.AccessKeyId {
		logging.Default().Fatal("access key in recording different than configuration")
	}
	aCred := new(model.Credential)
	aCred.AccessKeyId = accessKey
	aCred.AccessSecretKey = m.AccessSecretKey
	aCred.Type = m.CredentialType
	aCred.UserId = &m.UserId
	return aCred, nil

}

func (m *playBackMockConf) Authorize(req *auth.AuthorizationRequest) (*auth.AuthorizationResponse, error) {
	return &auth.AuthorizationResponse{true, nil}, nil
}