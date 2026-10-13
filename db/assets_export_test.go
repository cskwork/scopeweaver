package db

import (
	"strconv"
	"testing"
	"time"

	"slices"
)

// fresh task, returning their ids. Unique stamp keeps concurrent runs isolated.
func exportFixture(t *testing.T, d *DB, as *AssetStore, stamp int64) (taskID int64, ids []int64) {
	t.Helper()
	taskID = 900000 + stamp%100000
	root, err := as.UpsertRootDomain(UpsertRootDomainReq{Domain: "export-root.example", TaskID: taskID})
	if err != nil {
		t.Fatal(err)
	}
	sub, err := as.UpsertSubdomain(UpsertSubdomainReq{Domain: "export-sub.export-root.example", RecordType: "A", RecordValue: []string{"203.0.113.77"}, TaskID: taskID})
	if err != nil {
		t.Fatal(err)
	}
	ip, err := as.UpsertIP(UpsertIPReq{IP: "203.0.113.77", BoundDomains: []string{"export-sub.export-root.example"}, TaskID: taskID})
	if err != nil {
		t.Fatal(err)
	}
	ids = []int64{root, sub, ip}
	t.Cleanup(func() {
		for _, id := range ids {
			d.Exec(`DELETE FROM assets WHERE id = $1`, id)
		}
	})
	return taskID, ids
}

func TestListAssetsForExportSelected(t *testing.T) {
	d, as, _ := testSetup(t)
	defer d.Close()
	_, ids := exportFixture(t, d, as, time.Now().UnixNano()%1000000)

	got, err := as.ListAssetsForExport(AssetExportFilter{}, ids)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(ids) {
		t.Fatalf("selected export = %d rows, want %d", len(got), len(ids))
	}
	gotIDs := map[int64]bool{}
	for _, a := range got {
		gotIDs[a.ID] = true
	}
	for _, id := range ids {
		if !gotIDs[id] {
			t.Fatalf("selected id %d missing from export", id)
		}
	}
}

func TestListAssetsForExportFilteredByTask(t *testing.T) {
	d, as, _ := testSetup(t)
	defer d.Close()
	taskID, ids := exportFixture(t, d, as, time.Now().UnixNano()%1000000)

	got, err := as.ListAssetsForExport(AssetExportFilter{TaskID: taskID}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(ids) {
		t.Fatalf("task export = %d rows, want %d", len(got), len(ids))
	}
	for _, a := range got {
		if !slices.Contains(a.TaskIDs, taskID) {
			t.Fatalf("asset %d not scoped to task %d: %v", a.ID, taskID, a.TaskIDs)
		}
	}
}

func TestListAssetsForExportFilteredByType(t *testing.T) {
	d, as, _ := testSetup(t)
	defer d.Close()
	taskID, _ := exportFixture(t, d, as, time.Now().UnixNano()%1000000)

	got, err := as.ListAssetsForExport(AssetExportFilter{TaskID: taskID, Type: "subdomain"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Type != "subdomain" {
		t.Fatalf("type filter returned %+v", got)
	}
}

func TestListAssetsForExportFilteredByDSL(t *testing.T) {
	d, as, _ := testSetup(t)
	defer d.Close()
	taskID, _ := exportFixture(t, d, as, time.Now().UnixNano()%1000000)

	got, err := as.ListAssetsForExport(AssetExportFilter{TaskID: taskID, DSL: "domain=export-sub"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Type != "subdomain" {
		t.Fatalf("dsl filter returned %+v", got)
	}
}

func TestListAssetsForExportFilteredByCompany(t *testing.T) {
	d, as, cs := testSetup(t)
	defer d.Close()
	stamp := time.Now().UnixNano() % 1000000
	companyID, _, _, _, _, err := cs.CreateCompanyWithScope(
		"Export Co "+strconv.FormatInt(stamp, 10), "",
		[]ScopeInput{{Kind: "domain", Value: "export-root.example"}}, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cs.DeleteCompany(companyID) }()

	taskID, _ := exportFixture(t, d, as, stamp)
	if err := cs.RecomputeAttribution(); err != nil {
		t.Fatal(err)
	}

	got, err := as.ListAssetsForExport(AssetExportFilter{CompanyID: companyID}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Fatal("company export returned no rows")
	}
	for _, a := range got {
		if a.CompanyID == nil || *a.CompanyID != companyID {
			t.Fatalf("asset %d not attributed to company %d", a.ID, companyID)
		}
	}
	_ = taskID
}

func TestListAssetsForExportEmptyResult(t *testing.T) {
	d, as, _ := testSetup(t)
	defer d.Close()
	got, err := as.ListAssetsForExport(AssetExportFilter{TaskID: 999999999}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty result, got %d rows", len(got))
	}
}

func TestListAssetsForExportRejectsBadDSL(t *testing.T) {
	d, as, _ := testSetup(t)
	defer d.Close()
	_, err := as.ListAssetsForExport(AssetExportFilter{DSL: `nope_field = "x"`}, nil)
	if err == nil {
		t.Fatal("expected an error for invalid DSL, got nil")
	}
}
