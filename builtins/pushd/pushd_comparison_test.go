package pushd_test

import (
	"testing"

	_ "github.com/mark3labs/go-bash/builtins/pushd"
	"github.com/mark3labs/go-bash/internal/cmpfixture"
)

func TestComparisonFixtures(t *testing.T) {
	cmpfixture.RunDir(t, "../../internal/testdata/fixtures/pushd")
}
