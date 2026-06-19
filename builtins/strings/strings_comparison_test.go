package strings_test

import (
	"testing"

	_ "github.com/mark3labs/go-bash/builtins/strings"
	"github.com/mark3labs/go-bash/internal/cmpfixture"
)

func TestComparisonFixtures(t *testing.T) {
	cmpfixture.RunDir(t, "../../internal/testdata/fixtures/strings")
}
