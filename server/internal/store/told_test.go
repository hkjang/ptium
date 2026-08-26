package store

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

// What the API answers with has to be what the API says it answers with.
//
// The admin overview returned three fields the schema did not list — how long
// the oldest generation has waited, how many failed in a day, when a deck was
// last finished — and the schema says additionalProperties: false, so a client
// validating the response would refuse a perfectly good answer. Worse, the
// three missing ones are the numbers an operator watches: a queue of twelve
// means nothing without the age of the oldest thing in it.
func TestTheAdminOverviewAnswersWithWhatItDocuments(t *testing.T) {
	schema, err := os.ReadFile("../../../api/openapi.yaml")
	if err != nil {
		t.Fatalf("openapi.yaml: %v", err)
	}
	said := string(schema)
	start := strings.Index(said, "    AdminOverview:")
	if start < 0 {
		t.Fatal("the schema no longer has an AdminOverview")
	}
	block := said[start : start+strings.Index(said[start:], "\n    AdminOverviewEnvelope:")]
	fields := reflect.TypeOf(Overview{})
	for index := 0; index < fields.NumField(); index++ {
		tag := strings.Split(fields.Field(index).Tag.Get("json"), ",")[0]
		if tag == "" || tag == "-" {
			continue
		}
		if !strings.Contains(block, "\n        "+tag+":") {
			t.Errorf("the overview answers with %q and the schema does not list it", tag)
		}
	}
}
