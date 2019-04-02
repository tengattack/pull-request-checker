package checker

import (
	"io/ioutil"
	"os"
	"path"
	"reflect"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tengattack/unified-ci/config"
	"sourcegraph.com/sourcegraph/go-diff/diff"
)

// CheckAnnotation contains path & position for github comment
type CheckAnnotation struct {
	Messages  []string // regexp format for comment message
	Path      string
	StartLine int
}

// TestsData contains the meta-data for a sub-test.
type TestsData struct {
	Language     string
	TestRepoPath string
	FileName     string
	Annotations  []CheckAnnotation
}

var dataSet = []TestsData{
	{"CPP", "../testdata", "sillycode.cpp", []CheckAnnotation{
		CheckAnnotation{[]string{`two`}, "sillycode.cpp", 5},
		CheckAnnotation{[]string{`explicit`}, "sillycode.cpp", 80},
	}},
	{"Go", "../testdata", "test1.go", []CheckAnnotation{
		CheckAnnotation{[]string{`\n\+\s*"bytes"`}, "test1.go", 3},
		CheckAnnotation{[]string{`\n\-\s*"bytes"`}, "test1.go", 6},
	}},
	{"Markdown", "../testdata/markdown", "hello ☺.md", []CheckAnnotation{
		{[]string{"Hello 你好"}, "hello ☺.md", 1},
		{[]string{"undefined"}, "hello ☺.md", 3},
	}},
}

func TestGenerateComments(t *testing.T) {
	Conf = config.BuildDefaultConf()

	err := InitLog()
	require.Nil(t, err)

	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok)
	currentDir := path.Dir(filename)

	for _, v := range dataSet {
		v := v
		t.Run(v.Language, func(t *testing.T) {
			t.Parallel()
			assert := assert.New(t)
			assert.NotNil(assert)
			require := require.New(t)
			require.NotNil(require)

			testRepoPath := path.Join(currentDir, v.TestRepoPath)
			out, err := ioutil.ReadFile(path.Join(testRepoPath, v.FileName+".diff"))
			require.NoError(err)
			logFilePath := path.Join(testRepoPath, v.FileName+".log")
			log, err := os.Create(logFilePath)
			require.NoError(err)
			defer os.Remove(logFilePath)
			defer log.Close()

			diffs, err := diff.ParseMultiFileDiff(out)
			require.NoError(err)

			lintEnabled := LintEnabled{}
			lintEnabled.Init(testRepoPath)

			annotations, problems, err := GenerateAnnotations(testRepoPath, diffs, lintEnabled, log)
			require.NoError(err)
			require.Equal(len(v.Annotations), problems)
			for i, check := range v.Annotations {
				assert.Equal(check.StartLine, *annotations[i].StartLine)
				assert.Equal(check.Path, *annotations[i].Path)
				for _, regexMessage := range check.Messages {
					assert.Regexp(regexMessage, *annotations[i].Message)
				}
			}
		})
	}
}

func TestGetTests(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	_, file, _, ok := runtime.Caller(0)
	require.True(ok)

	tests := getTests(path.Join(path.Dir(file), "../"))
	assert.True(reflect.DeepEqual(tests, map[string][]string{"go": []string{"go test ./..."}}))
}
