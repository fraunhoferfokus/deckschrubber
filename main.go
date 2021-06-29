package main

import (
	"crypto/tls"
	"net/http"
	"sort"
	"strings"
	"syscall"
	"time"

	"encoding/json"
	"net/url"

	"flag"

	"io"

	"fmt"
	"os"

	"regexp"

	"golang.org/x/crypto/ssh/terminal"

	"github.com/docker/distribution/context"
	schema2 "github.com/docker/distribution/manifest/schema2"
	"github.com/docker/distribution/reference"
	"github.com/docker/distribution/registry/client"
	"github.com/docker/distribution/registry/client/auth"
	"github.com/docker/distribution/registry/client/auth/challenge"
	"github.com/docker/distribution/registry/client/transport"
	"github.com/fraunhoferfokus/deckschrubber/util"
	log "github.com/sirupsen/logrus"
)

var (
	/** CLI flags */
	// Base URL of registry
	registryURL *string
	// Regexps for filtering repositories and tags
	repoRegexpStr, tagRegexpStr, negTagRegexpStr *string
	// Maximum age of image to consider for deletion
	day, month, year *int
	// Max number of repositories to be fetched from registry
	repoCount *int
	// Number of the latest n matching images of an repository that will be ignored
	latest *int
	// If true, application runs in debug mode
	debug *bool
	// If true, no actual deletion is done
	dry *bool
	// If true, version is shown and program quits
	ver *bool

	// Compiled regexps
	repoRegexp, tagRegexp, negTagRegexp *regexp.Regexp
	// Skip insecure TLS
	insecure *bool
	// Username and password
	uname, passwd *string

	// Credential store
	cstore util.CStore
)

const (
	version string = "0.6.0"
)

func init() {
	/** CLI flags */
	// Max number of repositories to fetch from registry (default = 5)
	repoCount = flag.Int("repos", 5, "number of repositories to garbage collect")
	// Base URL of registry (default = http://localhost:5000)
	registryURL = flag.String("registry", "http://localhost:5000", "URL of registry")
	// Maximum age of iamges to consider for deletion in days (default = 0)
	day = flag.Int("day", 0, "max age in days")
	// Maximum age of months to consider for deletion in days (default = 0)
	month = flag.Int("month", 0, "max age in months")
	// Maximum age of iamges to consider for deletion in years (default = 0)
	year = flag.Int("year", 0, "max age in days")
	// Regexp for images (default = .*)
	repoRegexpStr = flag.String("repo", ".*", "matching repositories (allows regexp)")
	// Regexp for tags (default = .*)
	tagRegexpStr = flag.String("tag", ".*", "matching tags (allows regexp)")
	// Negative regexp for tags (default = empty)
	negTagRegexpStr = flag.String("ntag", "", "non matching tags (allows regexp)")
	// The number of the latest matching images of an repository that won't be deleted
	latest = flag.Int("latest", 1, "number of the latest matching images of an repository that won't be deleted")
	// Dry run option (doesn't actually delete)
	debug = flag.Bool("debug", false, "run in debug mode")
	// Dry run option (doesn't actually delete)
	dry = flag.Bool("dry", false, "does not actually deletes")
	// Shows version
	ver = flag.Bool("v", false, "shows version and quits")
	// Skip insecure TLS
	insecure = flag.Bool("insecure", false, "Skip insecure TLS verification")
	// Username and password
	uname = flag.String("user", "", "Username for basic authentication")
	passwd = flag.String("password", "", "Password for basic authentication")
}

func main() {
	flag.Parse()

	if len(os.Args) <= 1 {
		flag.Usage()
		return
	}

	// Compile regular expressions
	repoRegexp = regexp.MustCompile(*repoRegexpStr)
	tagRegexp = regexp.MustCompile(*tagRegexpStr)
	if *negTagRegexpStr != "" {
		negTagRegexp = regexp.MustCompile(*negTagRegexpStr)
	}

	if *ver {
		fmt.Printf("Version: %s\n", version)
		os.Exit(0)
	}

	if *debug {
		log.SetLevel(log.DebugLevel)
	}

	// Parse and verify registry URL
	regParsedURL, err := url.Parse(*registryURL)
	if err != nil {
		log.Fatalf("Could not parse registry URL! (error: %v)", err)
		os.Exit(1)
	}

	// Add basic auth if user/pass is provided
	if *uname != "" && *passwd == "" {
		fmt.Println("Password:")
		bytePassword, err := terminal.ReadPassword(int(syscall.Stdin))
		if err == nil {
			stringPassword := string(bytePassword[:])
			passwd = &stringPassword
		} else {
			fmt.Println("Could not read password. Quitting!")
			os.Exit(1)
		}
	}
	cstore.SetBasic(*uname, *passwd)

	baseTransport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: *insecure},
	}

	// API Check:
	// depending on what the endpoint answers, we might need to do some extra
	// steps for authentication (i.e., parse header, query endpoint, etc).
	// see: https://docs.docker.com/registry/spec/api/
	// Code snippets are taken from:
	// https://github.com/distribution/distribution/blob/ad8f5caba068b2d4b83ba5a1c3e9d74e1da2c16e/registry/client/auth/session.go#L74-L87
	log.Debug("Trying to fetch auth challenges")
	pingPath := regParsedURL.Path
	if v2Root := strings.Index(regParsedURL.Path, "/v2/"); v2Root != -1 {
		pingPath = pingPath[:v2Root+4]
	} else if v1Root := strings.Index(regParsedURL.Path, "/v1/"); v1Root != -1 {
		pingPath = pingPath[:v1Root] + "/v2/"
	} else {
		log.Warning("Could not figure out ping path (defaulting to '/v2/')")
		pingPath = "/v2/"
	}
	ping := url.URL{
		Host:   regParsedURL.Host,
		Scheme: regParsedURL.Scheme,
		Path:   pingPath,
	}
	log.Debugf("Ping path is '%s'", ping.String())

	manager := challenge.NewSimpleManager()
	httpClient := &http.Client{Transport: baseTransport}
	res, err := httpClient.Get(ping.String())
	if err != nil {
		log.Warningf("Could not query API for auth challenges! (err: %v)", err)
	}
	manager.AddResponse(res)

	// Prepare handlers for both token-based and basic authentication
	// @TODO: I'm not finding proper documentation for the scopes!
	tokenHandler := auth.NewTokenHandlerWithOptions(auth.TokenHandlerOptions{
		Transport:   baseTransport,
		Credentials: &cstore,
		ClientID:    "deckschrubber",
		Scopes: []auth.Scope{
			auth.RegistryScope{
				Name:    "catalog",
				Actions: []string{"*"},
			},
			auth.RepositoryScope{
				Repository: "*",
				// Actions:    []string{"push", "pull"}, <- is this platform dependent? (e.g., GitLab vs Docker Hub)
				Actions: nil,
			},
		},
		Logger: log.New(),
	})
	basicHandler := auth.NewBasicHandler(&cstore)
	transport := transport.NewTransport(baseTransport, auth.NewAuthorizer(manager, tokenHandler, basicHandler))

	// Create registry object
	r, err := client.NewRegistry(*registryURL, transport)
	if err != nil {
		log.Fatalf("Could not create registry object! (err: %s", err)
	}

	// List of all repositories fetched from the registry. The number
	// of fetched repositories depends on the number provided by the
	// user ('-repos' flag)
	entries := make([]string, *repoCount)

	// Empty context for all requests in sequel
	ctx := context.Background()

	// Fetch all repositories from the registry
	numFilled, err := r.Repositories(ctx, entries, "")
	if err != nil && err != io.EOF {
		log.Fatalf("Error while fetching repositories! (err: %v)", err)
	}
	log.WithFields(log.Fields{"count": numFilled, "entries": entries[:numFilled]}).Info("Successfully fetched repositories.")

	// Deadline defines the youngest creation date for an image
	// to be considered for deletion
	deadline := time.Now().AddDate(*year/-1, *month/-1, *day/-1)

	// Fetch information about images belonging to each repository
	for _, entry := range entries[:numFilled] {
		logger := log.WithField("repo", entry)

		matched := repoRegexp.MatchString(entry)

		if !matched {
			logger.WithFields(log.Fields{"entry": entry}).Debug("Ignore non matching repository (-repo=", *repoRegexpStr, ")")
			continue
		}

		// Establish repository object in registry
		repoName, err := reference.WithName(entry)

		if err != nil {
			logger.Fatalf("Could not parse repo from name! (err: %v)", err)
		}

		repo, err := client.NewRepository(repoName, *registryURL, transport)
		if err != nil {
			logger.WithFields(log.Fields{"entry": entry}).Fatalf("Could not create repo from name! (err: %v)", err)
		}
		logger.Debug("Successfully created repository object.")

		tagsService := repo.Tags(ctx)
		blobsService := repo.Blobs(ctx)
		manifestService, err := repo.Manifests(ctx)
		if err != nil {
			logger.Fatalf("Couldn't fetch manifest service! (err: %v)", err)
		}

		tagsData, err := tagsService.All(ctx)
		if err != nil {
			logger.Fatalf("Couldn't fetch tags! (err: %v)", err)
		}

		var tags []Image

		// Fetch information about each tag of a repository
		// This involves fetching the manifest, its details,
		// and the corresponding blob information
		tagFetchDataErrors := false
		for _, tag := range tagsData {
			tagLogger := logger.WithField("tag", tag)

			tagLogger.Debug("Fetching tag...")
			desc, err := tagsService.Get(ctx, tag)
			if err != nil {
				tagLogger.Error("Could not fetch tag!")
				tagFetchDataErrors = true
				break
			}

			tagLogger.Debug("Fetching manifest...")
			mnfst, err := manifestService.Get(ctx, desc.Digest)
			if err != nil {
				tagLogger.Error("Could not fetch manifest!")
				tagFetchDataErrors = true
				break
			}

			tagLogger.Debug("Parsing manifest details...")
			_, p, err := mnfst.Payload()
			if err != nil {
				tagLogger.Error("Could not parse manifest detail!")
				tagFetchDataErrors = true
				break
			}

			m := new(schema2.DeserializedManifest)
			m.UnmarshalJSON(p)

			tagLogger.Debug("Fetching blob")
			b, err := blobsService.Get(ctx, m.Manifest.Config.Digest)
			if err != nil {
				tagLogger.Error("Could not fetch blob!")
				tagFetchDataErrors = true
				break
			}

			var blobInfo BlobInfo
			json.Unmarshal(b, &blobInfo)

			tags = append(tags, Image{entry, tag, blobInfo.Created, desc})
		}

		if tagFetchDataErrors {
			// In case of error at any one tag, skip entire repo
			// (avoid acting on incomplete data, which migth lead to
			// deleting images that are actually in use)
			logger.Error("Error obtaining tag data - skipping this repo")
			continue
		}

		sort.Sort(ImageByDate(tags))

		logger.Debug("Analyzing tags...")

		tagCount := len(tags)

		if tagCount == 0 {
			logger.Debug("Ignore repository with no matching tags")
			continue
		}

		deletableTags := make(map[int]Image)
		nonDeletableTags := make(map[int]Image)

		ignoredTags := 0

		for tagIndex := len(tags) - 1; tagIndex >= 0; tagIndex-- {
			tag := tags[tagIndex]
			markForDeletion := false
			tagLogger := logger.WithField("tag", tag.Tag)

			// Provides a text which is followed by the tag and ntag flag values. The
			// latter iff defined.
			withTagParens := func(text string) string {
				xs := []string{fmt.Sprintf("-tag=%s", *tagRegexpStr)}
				if *negTagRegexpStr != "" {
					xs = append(xs, fmt.Sprintf("-ntag=%s", *negTagRegexpStr))
				}
				return fmt.Sprintf("%s (%s)", text, strings.Join(xs, ", "))
			}

			// Check whether the tag matches. If that's the case, don't stop there, and
			// check for the negative regexp as well.
			matched := tagRegexp.MatchString(tag.Tag)
			if matched && negTagRegexp != nil {
				negTagMatch := negTagRegexp.MatchString(tag.Tag)
				matched = !negTagMatch
			}

			if matched {
				tagLogger.Debug(withTagParens("Tag matches, considering for deletion"))
				if tag.Time.Before(deadline) {
					if ignoredTags < *latest {
						tagLogger.WithField("time", tag.Time).Infof("Ignore %d latest matching tags (-latest=%d)", *latest, *latest)
						ignoredTags++
					} else {
						tagLogger.WithField("tag", tag.Tag).WithField("time", tag.Time).Infof("Marking tag as outdated")
						markForDeletion = true
					}
				} else {
					tagLogger.Info("Tag not outdated")
					ignoredTags++
				}
			} else {
				tagLogger.Info(withTagParens("Ignore non matching tag"))
			}

			if markForDeletion {
				deletableTags[tagIndex] = tag
			} else {
				nonDeletableTags[tagIndex] = tag
			}
		}

		// This approach is actually a workaround for the problem that Docker
		// Distribution doesn't implement TagService.Untag operation at the time of
		// this writing.
		// Actually we have to delete the underlying image (specified via its Digest
		// value), taking care not to delete images that are referenced by tags which
		// we want to preserve
		nonDeletableDigests := make(map[string]string)
		for _, tag := range nonDeletableTags {
			if nonDeletableDigests[tag.Descriptor.Digest.String()] == "" {
				nonDeletableDigests[tag.Descriptor.Digest.String()] = tag.Tag
			} else {
				nonDeletableDigests[tag.Descriptor.Digest.String()] = nonDeletableDigests[tag.Descriptor.Digest.String()] + ", " + tag.Tag
			}
		}

		digestsDeleted := make(map[string]bool)
		for _, tag := range deletableTags {
			if !digestsDeleted[tag.Descriptor.Digest.String()] {
				if nonDeletableDigests[tag.Descriptor.Digest.String()] == "" {
					logger.WithField("tag", tag.Tag).Info("All tags for this image digest marked for deletion")
					if !*dry {
						logger.WithField("tag", tag.Tag).WithField("time", tag.Time).WithField("digest", tag.Descriptor.Digest).Infof("Deleting image (-dry=%v)", *dry)
						err := manifestService.Delete(context.Background(), tag.Descriptor.Digest)
						if err == nil {
							digestsDeleted[tag.Descriptor.Digest.String()] = true
						} else {
							logger.WithField("tag", tag.Tag).WithField("err", err).Error("Could not delete image!")
						}
					} else {
						logger.WithField("tag", tag.Tag).WithField("time", tag.Time).Infof("Not actually deleting image (-dry=%v)", *dry)
					}
				} else {
					logger.WithField("tag", tag.Tag).WithField("alsoUsedByTags", nonDeletableDigests[tag.Descriptor.Digest.String()]).Infof("The underlying image is also used by non-deletable tags - skipping deletion")
				}
			} else {
				logger.WithField("tag", tag.Tag).Debug("Image under tag already deleted")
			}
		}

	}
}
