package browse

import (
	"encoding/base64"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"sync"
	"testing"
	"testing/synctest"

	"github.com/cli/cli/v2/internal/config"
	fd "github.com/cli/cli/v2/internal/featuredetection"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/repo/view"
	"github.com/cli/cli/v2/pkg/extensions"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/search"
	"github.com/rivo/tview"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_getReadme(t *testing.T) {
	reg := httpmock.Registry{}
	defer reg.Verify(t)

	content := base64.StdEncoding.EncodeToString([]byte("lol"))

	reg.Register(
		httpmock.REST("GET", "repos/cli/gh-cool/readme"),
		httpmock.JSONResponse(view.RepoReadme{Content: content}))

	client := &http.Client{Transport: &reg}

	opts := ExtBrowseOpts{
		Rg: &readmeGetter{client: client},
	}

	content, err := getReadme(opts, "cli/gh-cool", 80)
	require.NoError(t, err)
	assert.Contains(t, content, "lol")
}

func Test_extList_loadSelectedReadme(t *testing.T) {
	for _, tt := range []struct {
		name           string
		olderError     bool
		currentError   bool
		clearSelection bool
		revisit        bool
		olderQueued    bool
	}{
		{name: "older success finishes last"},
		{name: "older error finishes last", olderError: true},
		{name: "current error is preserved", currentError: true},
		{name: "filter removes all entries", clearSelection: true},
		{name: "revisit the original extension", revisit: true},
		{name: "older result queued before selection changes", olderQueued: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				reg := &httpmock.Registry{}
				defer reg.Verify(t)
				releaseOlder := make(chan struct{})
				reg.Register(httpmock.REST("GET", "repos/cli/gh-first/readme"), func(req *http.Request) (*http.Response, error) {
					<-releaseOlder
					if tt.olderError {
						return nil, errors.New("older request failed")
					}
					return httpmock.JSONResponse(view.RepoReadme{
						Content: base64.StdEncoding.EncodeToString([]byte("older readme")),
					})(req)
				})
				if !tt.clearSelection {
					reg.Register(httpmock.REST("GET", "repos/cli/gh-second/readme"), func(req *http.Request) (*http.Response, error) {
						if tt.currentError {
							return nil, errors.New("current request failed")
						}
						return httpmock.JSONResponse(view.RepoReadme{
							Content: base64.StdEncoding.EncodeToString([]byte("current readme")),
						})(req)
					})
				}
				if tt.revisit {
					reg.Register(httpmock.REST("GET", "repos/cli/gh-first/readme"), httpmock.JSONResponse(view.RepoReadme{
						Content: base64.StdEncoding.EncodeToString([]byte("revisited readme")),
					}))
				}

				opts := ExtBrowseOpts{
					Rg:     &readmeGetter{client: &http.Client{Transport: reg}},
					Logger: log.New(io.Discard, "", 0),
				}
				el := newExtList(opts, uiRegistry{List: tview.NewList()}, []extEntry{
					{FullName: "cli/gh-first"},
					{FullName: "cli/gh-second"},
				})
				updates := make(chan func(), 1)
				el.QueueUpdateDraw = func(f func()) *tview.Application {
					updates <- f
					return nil
				}
				readme := tview.NewTextView()
				readme.SetRect(0, 0, 80, 20)
				el.loadSelectedReadme(readme)
				synctest.Wait()
				assert.Equal(t, "...fetching readme...", readme.GetText(true))
				var olderUpdate func()
				if tt.olderQueued {
					close(releaseOlder)
					olderUpdate = <-updates
				}

				if tt.clearSelection {
					el.Filter("no matching extension")
				} else {
					el.ScrollDown()
				}
				el.loadSelectedReadme(readme)
				if !tt.clearSelection {
					(<-updates)()
				}
				expected := "current readme"
				if tt.revisit {
					el.ScrollUp()
					el.loadSelectedReadme(readme)
					(<-updates)()
					expected = "revisited readme"
				}
				if tt.currentError || tt.clearSelection {
					expected = "unable to fetch readme :("
				}
				assert.Contains(t, readme.GetText(true), expected)
				currentText := readme.GetText(true)
				readme.ScrollTo(3, 0)

				if !tt.olderQueued {
					close(releaseOlder)
					olderUpdate = <-updates
				}
				olderUpdate()
				assert.Equal(t, currentText, readme.GetText(true))
				row, col := readme.GetScrollOffset()
				assert.Equal(t, 3, row)
				assert.Zero(t, col)
			})
		})
	}
}

func Test_getExtensionRepos(t *testing.T) {
	reg := httpmock.Registry{}
	defer reg.Verify(t)

	client := &http.Client{Transport: &reg}

	values := url.Values{
		"page":     []string{"1"},
		"per_page": []string{"100"},
		"q":        []string{"topic:gh-extension"},
	}
	cfg := config.NewMockConfig()

	cfg.AuthenticationFunc = func() gh.AuthConfig {
		authCfg := &config.AuthConfig{}
		authCfg.SetDefaultHost("github.com", "")
		return authCfg
	}

	reg.Register(
		httpmock.QueryMatcher("GET", "search/repositories", values),
		httpmock.JSONResponse(map[string]any{
			"incomplete_results": false,
			"total_count":        4,
			"items": []any{
				map[string]any{
					"name":        "gh-screensaver",
					"full_name":   "vilmibm/gh-screensaver",
					"description": "terminal animations",
					"owner": map[string]any{
						"login": "vilmibm",
					},
				},
				map[string]any{
					"name":        "gh-cool",
					"full_name":   "cli/gh-cool",
					"description": "it's just cool ok",
					"owner": map[string]any{
						"login": "cli",
					},
				},
				map[string]any{
					"name":        "gh-triage",
					"full_name":   "samcoe/gh-triage",
					"description": "helps with triage",
					"owner": map[string]any{
						"login": "samcoe",
					},
				},
				map[string]any{
					"name":        "gh-gei",
					"full_name":   "github/gh-gei",
					"description": "something something enterprise",
					"owner": map[string]any{
						"login": "github",
					},
				},
			},
		}),
	)

	searcher := search.NewSearcher(client, "github.com", &fd.DisabledDetectorMock{})
	emMock := &extensions.ExtensionManagerMock{}
	emMock.ListFunc = func() []extensions.Extension {
		return []extensions.Extension{
			&extensions.ExtensionMock{
				URLFunc: func() string {
					return "https://github.com/vilmibm/gh-screensaver"
				},
			},
			&extensions.ExtensionMock{
				URLFunc: func() string {
					return "https://github.com/github/gh-gei"
				},
			},
		}
	}

	opts := ExtBrowseOpts{
		Searcher: searcher,
		Em:       emMock,
		Cfg:      cfg,
	}

	extEntries, err := getExtensions(opts)
	assert.NoError(t, err)

	expectedEntries := []extEntry{
		{
			URL:         "https://github.com/vilmibm/gh-screensaver",
			Name:        "gh-screensaver",
			FullName:    "vilmibm/gh-screensaver",
			Installed:   true,
			Official:    false,
			description: "terminal animations",
		},
		{
			URL:         "https://github.com/cli/gh-cool",
			Name:        "gh-cool",
			FullName:    "cli/gh-cool",
			Installed:   false,
			Official:    true,
			description: "it's just cool ok",
		},
		{
			URL:         "https://github.com/samcoe/gh-triage",
			Name:        "gh-triage",
			FullName:    "samcoe/gh-triage",
			Installed:   false,
			Official:    false,
			description: "helps with triage",
		},
		{
			URL:         "https://github.com/github/gh-gei",
			Name:        "gh-gei",
			FullName:    "github/gh-gei",
			Installed:   true,
			Official:    true,
			description: "something something enterprise",
		},
	}

	assert.Equal(t, expectedEntries, extEntries)
}

func Test_extEntry(t *testing.T) {
	cases := []struct {
		name          string
		ee            extEntry
		expectedTitle string
		expectedDesc  string
	}{
		{
			name: "official",
			ee: extEntry{
				Name:        "gh-cool",
				FullName:    "cli/gh-cool",
				Installed:   false,
				Official:    true,
				description: "it's just cool ok",
			},
			expectedTitle: "cli/gh-cool [yellow](official)",
			expectedDesc:  "it's just cool ok",
		},
		{
			name: "no description",
			ee: extEntry{
				Name:        "gh-nodesc",
				FullName:    "barryburton/gh-nodesc",
				Installed:   false,
				Official:    false,
				description: "",
			},
			expectedTitle: "barryburton/gh-nodesc",
			expectedDesc:  "no description provided",
		},
		{
			name: "installed",
			ee: extEntry{
				Name:        "gh-screensaver",
				FullName:    "vilmibm/gh-screensaver",
				Installed:   true,
				Official:    false,
				description: "animations in your terminal",
			},
			expectedTitle: "vilmibm/gh-screensaver [green](installed)",
			expectedDesc:  "animations in your terminal",
		},
		{
			name: "neither",
			ee: extEntry{
				Name:        "gh-triage",
				FullName:    "samcoe/gh-triage",
				Installed:   false,
				Official:    false,
				description: "help with triage",
			},
			expectedTitle: "samcoe/gh-triage",
			expectedDesc:  "help with triage",
		},
		{
			name: "both",
			ee: extEntry{
				Name:        "gh-gei",
				FullName:    "github/gh-gei",
				Installed:   true,
				Official:    true,
				description: "something something enterprise",
			},
			expectedTitle: "github/gh-gei [yellow](official) [green](installed)",
			expectedDesc:  "something something enterprise",
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expectedTitle, tt.ee.Title())
			assert.Equal(t, tt.expectedDesc, tt.ee.Description())
		})
	}
}

func Test_extList(t *testing.T) {
	opts := ExtBrowseOpts{
		Logger: log.New(io.Discard, "", 0),
		Em: &extensions.ExtensionManagerMock{
			InstallFunc: func(repo ghrepo.Interface, _ string) error {
				assert.Equal(t, "cli/gh-cool", ghrepo.FullName(repo))
				return nil
			},
			RemoveFunc: func(name string) error {
				assert.Equal(t, "cool", name)
				return nil
			},
		},
	}
	cmdFlex := tview.NewFlex()
	app := tview.NewApplication()
	list := tview.NewList()
	pages := tview.NewPages()
	ui := uiRegistry{
		List:    list,
		App:     app,
		CmdFlex: cmdFlex,
		Pages:   pages,
	}
	extEntries := []extEntry{
		{
			Name:        "gh-cool",
			FullName:    "cli/gh-cool",
			Installed:   false,
			Official:    true,
			description: "it's just cool ok",
		},
		{
			Name:        "gh-screensaver",
			FullName:    "vilmibm/gh-screensaver",
			Installed:   true,
			Official:    false,
			description: "animations in your terminal",
		},
		{
			Name:        "gh-triage",
			FullName:    "samcoe/gh-triage",
			Installed:   false,
			Official:    false,
			description: "help with triage",
		},
		{
			Name:        "gh-gei",
			FullName:    "github/gh-gei",
			Installed:   true,
			Official:    true,
			description: "something something enterprise",
		},
	}

	extList := newExtList(opts, ui, extEntries)

	extList.QueueUpdateDraw = func(f func()) *tview.Application {
		f()
		return app
	}

	extList.WaitGroup = &sync.WaitGroup{}

	extList.Filter("cool")
	assert.Equal(t, 1, extList.ui.List.GetItemCount())

	title, _ := extList.ui.List.GetItemText(0)
	assert.Equal(t, "cli/gh-cool [yellow](official)", title)

	extList.InstallSelected()
	assert.True(t, extList.extEntries[0].Installed)

	// so I think the goroutines are causing a later failure because the toggleInstalled isn't seen.

	extList.Refresh()
	assert.Equal(t, 1, extList.ui.List.GetItemCount())

	title, _ = extList.ui.List.GetItemText(0)
	assert.Equal(t, "cli/gh-cool [yellow](official) [green](installed)", title)

	extList.RemoveSelected()
	assert.False(t, extList.extEntries[0].Installed)

	extList.Refresh()
	assert.Equal(t, 1, extList.ui.List.GetItemCount())

	title, _ = extList.ui.List.GetItemText(0)
	assert.Equal(t, "cli/gh-cool [yellow](official)", title)

	extList.Reset()
	assert.Equal(t, 4, extList.ui.List.GetItemCount())

	ee, ix := extList.FindSelected()
	assert.Equal(t, 0, ix)
	assert.Equal(t, "cli/gh-cool [yellow](official)", ee.Title())

	extList.ScrollDown()
	ee, ix = extList.FindSelected()
	assert.Equal(t, 1, ix)
	assert.Equal(t, "vilmibm/gh-screensaver [green](installed)", ee.Title())

	extList.ScrollUp()
	ee, ix = extList.FindSelected()
	assert.Equal(t, 0, ix)
	assert.Equal(t, "cli/gh-cool [yellow](official)", ee.Title())

	extList.PageDown()
	ee, ix = extList.FindSelected()
	assert.Equal(t, 3, ix)
	assert.Equal(t, "github/gh-gei [yellow](official) [green](installed)", ee.Title())

	extList.PageUp()
	ee, ix = extList.FindSelected()
	assert.Equal(t, 0, ix)
	assert.Equal(t, "cli/gh-cool [yellow](official)", ee.Title())
}
