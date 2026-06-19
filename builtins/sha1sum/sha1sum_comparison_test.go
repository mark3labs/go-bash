package sha1sum_test

import (
	"testing"

	_ "github.com/mark3labs/go-bash/builtins/sha1sum"
	"github.com/mark3labs/go-bash/internal/cmpfixture"
)

func TestComparisonFixtures(t *testing.T) {
	cmpfixture.RunDir(t, "../../internal/testdata/fixtures/sha1sum")
}
