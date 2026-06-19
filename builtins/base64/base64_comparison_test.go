package base64_test

import (
	"testing"

	_ "github.com/mark3labs/go-bash/builtins/base64"
	"github.com/mark3labs/go-bash/internal/cmpfixture"
)

func TestComparisonFixtures(t *testing.T) {
	cmpfixture.RunDir(t, "../../internal/testdata/fixtures/base64")
}
