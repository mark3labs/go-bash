package seq_test

import (
	"testing"

	_ "github.com/mark3labs/go-bash/builtins/seq"
	"github.com/mark3labs/go-bash/internal/cmpfixture"
)

func TestComparisonFixtures(t *testing.T) {
	cmpfixture.RunDir(t, "../../internal/testdata/fixtures/seq")
}
