package workspace

import (
	"errors"

	"github.com/occccad/occccad/internal/workbenchconfig"
	"testing"
)

func TestDocumentRegistryRequiresCompleteUniqueAdapters(t *testing.T) {
	part, err := documentAdapters.Lookup("PART")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = NewDocumentRegistry(part, part); err == nil {
		t.Fatal("duplicate accepted")
	}
	incomplete := part
	incomplete.Preview = nil
	if _, err = NewDocumentRegistry(incomplete); err == nil {
		t.Fatal("missing domain port accepted")
	}
	if _, err = documentAdapters.Lookup("UNREGISTERED"); !errors.Is(err, ErrValidation) {
		t.Fatalf("unsupported type: %v", err)
	}
	c, err := workbenchconfig.Read()
	if err != nil {
		t.Fatal(err)
	}
	if err = ValidateDocumentCatalog(c); err != nil {
		t.Fatal(err)
	}
	c.Documents[0].Adapter = "UNREGISTERED"
	if err = ValidateDocumentCatalog(c); err == nil {
		t.Fatal("missing adapter published")
	}
}
