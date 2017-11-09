package main

import (
	"sort"
	"strings"
	"time"

	"encoding/json"

	"flag"

	"io"

	"fmt"
	"os"

	"regexp"

	"crypto/tls"
	"net/http"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/distribution/context"
	schema2 "github.com/docker/distribution/manifest/schema2"
	"github.com/docker/distribution/reference"
	"github.com/docker/distribution/registry/client"
	"github.com/heroku/docker-registry-client/registry"
)

var (
	/** CLI flags */
	// Base URL of registry
	registryURL, username, password *string
	// Flags to filter repos/tags
	repoRegexp, tagRegexp, repos *string
	// Maximum number of repositories to fetch
	maxRepos *int
	// Maximum age of image to consider for deletion
	day, month, year *int
	// Number of the latest n matching images of an repository that will be ignored
	latest *int
	// If true, application runs in debug mode
	debug *bool
	// If true, no actual deletion is done
	dry *bool
	// If true, version is shown and program quits
	ver *bool
	// If true, https connection will ignore verification error
	insecure *bool
	// If ture, promote user to enter registry password
	enterPwd *bool
)

const (
	version string = "0.3.0"
	// Max repos: 65535
	maxRepoCount int = 65535
)

func init() {
	/** CLI flags */
	// Base URL of registry (default = http://localhost:5000)
	registryURL = flag.String("registry", "http://localhost:5000", "URL of registry")
	// registry username (default = "")
	username = flag.String("username", "", "registry username to login")
	// registry password (default = "")
	password = flag.String("password", "", "registry password to login")
	// Promote to enter password
	enterPwd = flag.Bool("promote", false, "promote to enter password")
	// Maximum age of iamges to consider for deletion in days (default = 0)
	day = flag.Int("day", 0, "max age in days")
	// Maximum age of months to consider for deletion in days (default = 0)
	month = flag.Int("month", 0, "max age in months")
	// Maximum age of iamges to consider for deletion in years (default = 0)
	year = flag.Int("year", 0, "max age in days")
	// Regexp for images (default = .*)
	repoRegexp = flag.String("repo_regexp", ".*", "matching repositories (allows regexp)")
	// images to check directly (default = "")
	repos = flag.String("repos", "", "matching repositories by name (allows mulitple value seperates by ,)")
	// max nubmer of repositories to garbage collect (default to no limit)
	maxRepos = flag.Int("max_repos", maxRepoCount, "max nubmer of repositories to garbage collect (default to 65535)")
	// Regexp for tags (default = .*)
	tagRegexp = flag.String("tag_regexp", ".*", "matching tags (allows regexp)")
	// The number of the latest matching images of an repository that won't be deleted
	latest = flag.Int("latest", 1, "number of the latest matching images of an repository that won't be deleted")
	// Dry run option (doesn't actually delete)
	debug = flag.Bool("debug", false, "run in debug mode")
	// Dry run option (doesn't actually delete)
	dry = flag.Bool("dry", false, "does not actually deletes")
	// https insecure flag
	insecure = flag.Bool("insecure", false, "ignore https verification error")
	// Shows version
	ver = flag.Bool("v", false, "shows version and quits")
}

func main() {
	flag.Parse()

	if *ver {
		fmt.Printf("Version: %s\n", version)
		os.Exit(0)
	}

	if *debug {
		log.SetLevel(log.DebugLevel)
	}

	if *enterPwd {
		fmt.Println("Enter registry login password:")
		fmt.Scanln(password)
	}

	// Empty context for all requests in sequel
	ctx := context.Background()
	transport := http.DefaultTransport
	if *insecure {
		transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		}
	}

	wrapTransport := registry.WrapTransport(transport, *registryURL,
		*username, *password)
	// Create registry object
	r, err := client.NewRegistry(ctx, *registryURL, wrapTransport)
	if err != nil {
		log.Fatalf("Could not create registry object! (err: %s", err)
	}

	fetchByRegexp := len(*repos) == 0

	// List of all repositories fetched from the registry.
	entries := make([]string, *maxRepos)
	numFilled := 0
	if fetchByRegexp {
		// Fetch all repositories from the registry
		numFilled, err = r.Repositories(ctx, entries, "")
		if err != nil && err != io.EOF {
			log.Fatalf("Error while fetching repositories! (err: %v)", err)
		}
		log.WithFields(log.Fields{"count": numFilled, "entries": entries[:numFilled]}).Info("Successfully fetched repositories.")
	} else {
		entries = strings.Split(*repos, ",")
		numFilled = len(entries)
	}

	repoImages := make(map[string][]Image)

	// Fetch information about images belonging to each repository
	for _, entry := range entries[:numFilled] {
		logger := log.WithField("repo", entry)

		if fetchByRegexp && strings.Compare(*repoRegexp, ".*") != 0 {
			matched, _ := regexp.MatchString(*repoRegexp, entry)

			if matched == false {
				logger.Debug("Ignore non matching repository (-repo_regexp=", *repoRegexp, ")")
				continue
			}
		}

		// Establish repository object in registry
		repoName, err := reference.WithName(entry)

		if err != nil {
			logger.Fatalf("Could not parse repo from name! (err: %v)", err)
		}

		repo, err := client.NewRepository(ctx, repoName, *registryURL, wrapTransport)
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

		tags, err := tagsService.All(ctx)
		if err != nil {
			logger.Fatalf("Couldn't fetch tags! (err: %v)", err)
		}

		var images []Image

		// Fetch information about each tag of a repository
		// This involves fetching the manifest, its details,
		// and the corresponding blob information
		for _, tag := range tags {
			tagLogger := logger.WithField("tag", tag)

			matched, _ := regexp.MatchString(*tagRegexp, tag)

			if !matched {
				tagLogger.Debug("Ignore non matching tag (-tag=", *tagRegexp, ")")
				continue
			}

			tagLogger.Debug("Fetching tag...")
			desc, err := tagsService.Get(ctx, tag)
			if err != nil {
				tagLogger.Error("Could not fetch tag!")
				continue
			}

			tagLogger.Debug("Fetching manifest...")
			mnfst, err := manifestService.Get(ctx, desc.Digest)
			if err != nil {
				tagLogger.Error("Could not fetch manifest!")
				continue
			}

			tagLogger.Debug("Parsing manifest details...")
			_, p, err := mnfst.Payload()
			if err != nil {
				tagLogger.Error("Could not parse manifest detail!")
				continue
			}

			m := new(schema2.DeserializedManifest)
			m.UnmarshalJSON(p)

			tagLogger.Debug("Fetching blob")
			b, err := blobsService.Get(ctx, m.Manifest.Config.Digest)
			if err != nil {
				tagLogger.Error("Could not fetch blob!")
				continue
			}

			var blobInfo BlobInfo
			json.Unmarshal(b, &blobInfo)

			images = append(images,
				Image{entry, tag, desc.Digest.String(), blobInfo.Created,
					func() error { return manifestService.Delete(ctx, desc.Digest) }})
		}

		sort.Sort(ImageByDate(images))
		repoImages[entry] = images
	}

	// Deadline defines the youngest creation date for an image
	// to be considered for deletion
	deadline := time.Now().AddDate(*year/-1, *month/-1, *day/-1)

	for name, tags := range repoImages {
		logger := log.WithField("repo", name)

		logger.Debug("Analyzing tags...")

		tagCount := len(tags)

		if tagCount == 0 {
			logger.Debug("Ignore repository with no matching tags")
			continue
		}
		latestDigests := make(map[string]bool)
		var latestStart int
		if *latest > len(tags) {
			latestStart = 0
		} else {
			latestStart = len(tags) - *latest
		}
		for _, tag := range tags[latestStart:] {
			latestDigests[tag.Digest] = true
		}

		for tagIndex, tag := range tags[:tagCount] {
			tagLogger := logger.WithField("tag", tag)
			if tagIndex > tagCount-1-*latest {
				tagLogger.WithField("time", tag.Time).Infof("Ignore %d latest matching images (-latest=%d)", tagIndex+1, *latest)
				continue
			}

			if tag.Time.Before(deadline) {
				tagLogger.WithField("time", tag.Time).Infof("Delete outdated image (-dry=%v)", *dry)

				if !*dry {
					if latestDigests[tag.Digest] {
						tagLogger.WithField("digest", tag.Digest).Info("Duplicate with lastest matching images, cannot delete image skip.")
					} else {
						err := tag.Delete()
						if err != nil {
							tagLogger.Error("Could not delete image!")
						}
					}

				}
			} else {
				tagLogger.Debug("Image not outdated")
			}
		}
	}
}
