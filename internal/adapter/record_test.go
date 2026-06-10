package adapter

import (
	"encoding/json"
	"strings"
	"testing"
)

// validRecord returns a minimal record that satisfies Validate.
func validRecord() Record {
	return Record{
		Schema:     SchemaV1,
		Source:     Source{Kind: "demo", Name: "demo"},
		Collection: Collection{ExternalID: "demo:collection", Kind: "source_collection", Name: "demo:collection"},
		Item:       Item{ExternalID: "demo:item:1", Kind: "message", Text: "hello"},
		Artifacts:  []Artifact{},
		Links:      []Link{},
		Relations:  []Relation{},
		Raw:        RawRef{Format: "json"},
	}
}

func TestValidateAcceptsMinimalRecord(t *testing.T) {
	if err := validRecord().Validate(); err != nil {
		t.Fatalf("valid record rejected: %v", err)
	}
}

func TestValidateRejectsMissingFields(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Record)
		want   string
	}{
		{"bad schema", func(r *Record) { r.Schema = "other.v1" }, "unsupported schema"},
		{"missing source kind", func(r *Record) { r.Source.Kind = "" }, "missing source.kind"},
		{"missing collection external_id", func(r *Record) { r.Collection.ExternalID = "" }, "missing collection.external_id"},
		{"missing collection kind", func(r *Record) { r.Collection.Kind = "" }, "missing collection.kind"},
		{"missing item external_id", func(r *Record) { r.Item.ExternalID = "" }, "missing item.external_id"},
		{"missing item kind", func(r *Record) { r.Item.Kind = "" }, "missing item.kind"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := validRecord()
			tc.mutate(&rec)
			err := rec.Validate()
			if err == nil {
				t.Fatalf("expected validation error for %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tc.want)
			}
		})
	}
}

func TestParseRoundTripsAndValidates(t *testing.T) {
	rec := validRecord()
	rec.Item.Metadata = json.RawMessage(`{"k":"v"}`)
	b, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := Parse(b)
	if err != nil {
		t.Fatalf("Parse failed on valid record: %v", err)
	}
	if parsed.Schema != SchemaV1 || parsed.Item.ExternalID != rec.Item.ExternalID {
		t.Fatalf("round trip mismatch: %#v", parsed)
	}
	// Unknown preserves the original bytes for downstream raw access.
	if len(parsed.Unknown) == 0 {
		t.Fatalf("Parse did not retain raw bytes")
	}
}

func TestParseRejectsInvalidJSON(t *testing.T) {
	if _, err := Parse([]byte(`{"schema":`)); err == nil {
		t.Fatalf("expected error on malformed JSON")
	}
}

func TestParseRejectsValidJSONFailingSchema(t *testing.T) {
	// Well-formed JSON, but the schema field is wrong.
	_, err := Parse([]byte(`{"schema":"nope","source":{"kind":"x"}}`))
	if err == nil {
		t.Fatalf("expected schema validation error")
	}
	if !strings.Contains(err.Error(), "unsupported schema") {
		t.Fatalf("error = %q, want unsupported schema", err.Error())
	}
}

func TestSchemaConstantIsStable(t *testing.T) {
	if SchemaV1 != "miseledger.adapter.v1" {
		t.Fatalf("SchemaV1 = %q, schema version changed unexpectedly", SchemaV1)
	}
}
