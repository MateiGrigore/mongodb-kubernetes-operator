package replica_set_remove_user

import (
	"context"
	"fmt"
	v1 "github.com/mongodb/mongodb-kubernetes-operator/api/v1"
	"github.com/mongodb/mongodb-kubernetes-operator/pkg/automationconfig"
	e2eutil "github.com/mongodb/mongodb-kubernetes-operator/test/e2e"
	"github.com/mongodb/mongodb-kubernetes-operator/test/e2e/mongodbtests"
	"github.com/mongodb/mongodb-kubernetes-operator/test/e2e/setup"
	. "github.com/mongodb/mongodb-kubernetes-operator/test/e2e/util/mongotester"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	code, err := e2eutil.RunTest(m)
	if err != nil {
		fmt.Println(err)
	}
	os.Exit(code)
}

func intPtr(x int) *int       { return &x }
func strPtr(s string) *string { return &s }

func TestCleanupUsers(t *testing.T) {
	ctx := context.Background()
	testCtx := setup.Setup(ctx, t)
	defer testCtx.Teardown()

	mdb, user := e2eutil.NewTestMongoDB(testCtx, "mdb0", "")

	_, err := setup.GeneratePasswordForUser(testCtx, user, "")
	if err != nil {
		t.Fatal(err)
	}

	lcr := automationconfig.CrdLogRotate{
		// fractional values are supported
		SizeThresholdMB: "0.1",
		LogRotate: automationconfig.LogRotate{
			TimeThresholdHrs:                1,
			NumUncompressed:                 10,
			NumTotal:                        10,
			IncludeAuditLogsWithMongoDBLogs: false,
		},
		PercentOfDiskspace: "1",
	}

	systemLog := automationconfig.SystemLog{
		Destination: automationconfig.File,
		Path:        "/tmp/mongod.log",
		LogAppend:   false,
	}

	// logRotate can only be configured if systemLog to file has been configured
	mdb.Spec.AgentConfiguration.LogRotate = &lcr
	mdb.Spec.AgentConfiguration.SystemLog = &systemLog

	// config member options
	memberOptions := []automationconfig.MemberOptions{
		{
			Votes:    intPtr(1),
			Tags:     map[string]string{"foo1": "bar1"},
			Priority: strPtr("1.5"),
		},
		{
			Votes:    intPtr(1),
			Tags:     map[string]string{"foo2": "bar2"},
			Priority: strPtr("1"),
		},
		{
			Votes:    intPtr(1),
			Tags:     map[string]string{"foo3": "bar3"},
			Priority: strPtr("2.5"),
		},
	}
	mdb.Spec.MemberConfig = memberOptions

	settings := map[string]interface{}{
		"electionTimeoutMillis": float64(20),
	}
	mdb.Spec.AutomationConfigOverride = &v1.AutomationConfigOverride{
		ReplicaSet: v1.OverrideReplicaSet{Settings: v1.MapWrapper{Object: settings}},
	}

	tester, err := FromResource(ctx, t, mdb)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("Create MongoDB Resource", mongodbtests.CreateMongoDBResource(&mdb, testCtx))
	t.Run("Basic tests", mongodbtests.BasicFunctionality(ctx, &mdb))
	t.Run("Keyfile authentication is configured", tester.HasKeyfileAuth(3))
	t.Run("AutomationConfig has the correct logRotateConfig", mongodbtests.AutomationConfigHasLogRotationConfig(ctx, &mdb, &lcr))
	t.Run("Test Basic Connectivity", tester.ConnectivitySucceeds())
	t.Run("Test SRV Connectivity", tester.ConnectivitySucceeds(WithURI(mdb.MongoSRVURI("")), WithoutTls(), WithReplicaSet(mdb.Name)))
	deletedUser := mdb.Spec.Users[0]
	t.Run("Delete user from MongoDB Resource", mongodbtests.RemoveAllUsersFromResource(ctx, &mdb))
	t.Run("MongoDB reaches Pending phase", mongodbtests.MongoDBReachesPendingPhase(ctx, &mdb))
	t.Run("Removed users are added to automation config", mongodbtests.AuthUsersDeletedIsUpdated(ctx, &mdb, deletedUser))
	t.Run("MongoDB reaches Running phase", mongodbtests.MongoDBReachesRunningPhase(ctx, &mdb))
	t.Run("Connection string secrets are cleaned up", mongodbtests.ConnectionStringSecretIsCleanedUp(ctx, &mdb, deletedUser.GetConnectionStringSecretName(mdb.Name)))
	t.Run("Delete MongoDB Resource", mongodbtests.DeleteMongoDBResource(&mdb, testCtx))
}
