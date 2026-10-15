package main

import (
	"context"
	"flag"
	"os"
	"testing"

	"github.com/robertguss/go-islands-starter/internal/testdb"
)

func TestBootstrapOnlyOnceWithoutWorkflowAccess(t *testing.T) {
	_, fixture := testdb.New(t)
	ctx := context.Background()
	if _, err := fixture.Exec(ctx, "DELETE FROM app.users WHERE id=$1", testdb.Admin); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MIGRATION_DATABASE_URL", fixture.Config().ConnString())
	originalArgs, originalFlags := os.Args, flag.CommandLine
	t.Cleanup(func() { os.Args = originalArgs; flag.CommandLine = originalFlags })
	invoke := func() error {
		flag.CommandLine = flag.NewFlagSet("admin-test", flag.ContinueOnError)
		os.Args = []string{"admin", "-email", "first@example.test", "-name", "First Administrator"}
		return run()
	}
	if err := invoke(); err != nil {
		t.Fatal(err)
	}
	var administrator, requester, reviewer bool
	if err := fixture.QueryRow(ctx, "SELECT access_admin,requester,reviewer FROM app.users WHERE email='first@example.test'").Scan(&administrator, &requester, &reviewer); err != nil {
		t.Fatal(err)
	}
	if !administrator || requester || reviewer {
		t.Fatal("bootstrap granted incorrect access")
	}
	if invoke() == nil {
		t.Fatal("bootstrap created a second administrator")
	}
}
