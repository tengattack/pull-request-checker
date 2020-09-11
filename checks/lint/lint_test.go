package lint

import (
	"context"
	"io/ioutil"
	"os"
	"path"
	"runtime"
	"strings"
	"testing"

	"github.com/sourcegraph/go-diff/diff"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tengattack/unified-ci/common"
	"github.com/tengattack/unified-ci/config"
)

func TestMain(m *testing.M) {
	common.Conf = config.BuildDefaultConf()
	err := common.InitLog(common.Conf)
	if err != nil {
		panic(err)
	}

	code := m.Run()
	os.Exit(code)
}

func TestLintRepo1(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	_, filename, _, ok := runtime.Caller(0)
	require.True(ok)

	currentDir := path.Dir(filename)
	repoDir := path.Join(currentDir, "../../testdata/go")

	fileName := "check.go"
	out, err := ioutil.ReadFile(path.Join(repoDir, fileName+".diff"))
	require.NoError(err)

	diffs, err := diff.ParseMultiFileDiff(out)
	require.NoError(err)

	lintEnabled := LintEnabled{}
	lintEnabled.Init(repoDir)
	common.Conf.Core.GolangCILint = "golangci-lint"

	var buf strings.Builder
	_, annotations, problems, err := LintRepo(context.TODO(), common.GithubRef{}, repoDir, diffs, lintEnabled, &buf)
	require.NoError(err)
	assert.NotEmpty(annotations)
	assert.NotZero(problems)
}

func TestLintRepo2(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	_, filename, _, ok := runtime.Caller(0)
	require.True(ok)

	currentDir := path.Dir(filename)
	repoDir := path.Join(currentDir, "../../testdata/Android")

	fileName := "app/src/main/AndroidManifest.xml"
	out, err := ioutil.ReadFile(path.Join(repoDir, fileName+".diff"))
	require.NoError(err)

	diffs, err := diff.ParseMultiFileDiff(out)
	require.NoError(err)

	lintEnabled := LintEnabled{}
	lintEnabled.Init(repoDir)
	if runtime.GOOS == "windows" {
		common.Conf.Core.AndroidLint = "gradlew.bat lint"
	} else {
		common.Conf.Core.AndroidLint = "./gradlew lint"
	}

	var buf strings.Builder
	_, annotations, problems, err := LintRepo(context.TODO(), common.GithubRef{}, repoDir, diffs, lintEnabled, &buf)
	require.NoError(err)
	assert.NotEmpty(annotations)
	assert.NotZero(problems)
}

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
	{"CPP", "../../testdata", "sillycode.cpp", []CheckAnnotation{
		{[]string{`two`}, "sillycode.cpp", 5},
		{[]string{`explicit`}, "sillycode.cpp", 80},
	}},
	{"Go", "../../testdata", "test1.go", []CheckAnnotation{
		{[]string{`\n\+\s*"bytes"`}, "test1.go", 3},
		{[]string{`\n\-\s*"bytes"`}, "test1.go", 6},
	}},
	{"Markdown", "../../testdata/markdown", "hello ☺.md", []CheckAnnotation{
		{[]string{"Hello 你好"}, "hello ☺.md", 1},
		{[]string{"undefined"}, "hello ☺.md", 3},
	}},
	{"Objective-C", "../../testdata/Objective-C", "sample.m", []CheckAnnotation{
		{[]string{`pool = `}, "sample.m", 3},
		{[]string{`NSLog\(`}, "sample.m", 6},
		{[]string{`\+\s+return 0;`}, "sample.m", 8},
	}},
}

func TestLintIndividually(t *testing.T) {
	common.Conf = config.BuildDefaultConf()

	err := common.InitLog(common.Conf)
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

			annotations, problems, err := LintIndividually(context.TODO(), common.GithubRef{}, testRepoPath, diffs, lintEnabled, nil, log)
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

func TestLintFileMode(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	_, filename, _, ok := runtime.Caller(0)
	require.True(ok)

	currentDir := path.Dir(filename)
	repoDir := path.Join(currentDir, "../../testdata/src")
	fileName := "src.diff"
	out, err := ioutil.ReadFile(path.Join(repoDir, fileName))
	require.NoError(err)

	diffs, err := diff.ParseMultiFileDiff(out)
	require.NoError(err)

	var buf strings.Builder
	lints, problems, err := LintFileMode(context.TODO(), common.GithubRef{}, repoDir, diffs, &buf)
	assert.NoError(err)
	assert.Equal(4, problems)
	for _, v := range lints {
		switch v.GetPath() {
		case "c.sh":
			assert.Equal(fileModeCheckShellScript, v.GetMessage())
		case "d.sh":
			assert.Equal(shebangCheckShellScript, v.GetMessage())
		case "h.py":
			assert.Equal(fileModeCheckExecutable, v.GetMessage())
		case "b.py":
			assert.Equal(fileModeCheckNormal, v.GetMessage())
		default:
			assert.Fail(v.GetPath() + " should not be ok")
		}
	}
}
