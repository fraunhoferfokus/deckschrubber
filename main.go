package main

import (
	"sort"
	"time"

	"encoding/json"

	"flag"

	"io"

	"fmt"
	"os"

	"regexp"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/distribution/context"
	schema2 "github.com/docker/distribution/manifest/schema2"
	"github.com/docker/distribution/reference"
	"github.com/docker/distribution/registry/client"
)

var (
	/** CLI flags */
	// Base URL of registry
	registryURL *string
	// Regexps for filtering repositories and tags
	repoRegexp, tagRegexp *string
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
)

const (
	version string = "0.4.0"
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
	repoRegexp = flag.String("repo", ".*", "matching repositories (allows regexp)")
	// Regexp for tags (default = .*)
	tagRegexp = flag.String("tag", ".*", "matching tags (allows regexp)")
	// The number of the latest matching images of an repository that won't be deleted
	latest = flag.Int("latest", 1, "number of the latest matching images of an repository that won't be deleted")
	// Dry run option (doesn't actually delete)
	debug = flag.Bool("debug", false, "run in debug mode")
	// Dry run option (doesn't actually delete)
	dry = flag.Bool("dry", false, "does not actually deletes")
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

	// Create registry object
	r, err := client.NewRegistry(*registryURL, nil)
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
	log.WithFields(log.Fields{"count": numFilled, "entries": entries}).Info("Successfully fetched repositories.")

	// Deadline defines the youngest creation date for an image
	// to be considered for deletion
	deadline := time.Now().AddDate(*year/-1, *month/-1, *day/-1)

	// Fetch information about images belonging to each repository
	for _, entry := range entries[:numFilled] {
		logger := log.WithField("repo", entry)

		matched, err := regexp.MatchString(*repoRegexp, entry)

		if matched == false {
			logger.WithFields(log.Fields{"entry": entry}).Debug("Ignore non matching repository (-repo=", *repoRegexp, ")")
			continue
		}

		// Establish repository object in registry
		repoName, err := reference.WithName(entry)

		if err != nil {
			logger.Fatalf("Could not parse repo from name! (err: %v)", err)
		}

		repo, err := client.NewRepository(repoName, *registryURL, nil)
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

		deletableTags := make(map[int] Image);
		nonDeletableTags := make(map[int] Image);

		ignoredTags := 0

		for tagIndex := len(tags)-1; tagIndex >= 0; tagIndex-- {
			tag := tags[tagIndex]
			markForDeletion := false;
			tagLogger := logger.WithField("tag", tag.Tag)

			matched, _ := regexp.MatchString(*tagRegexp, tag.Tag)

			if matched {
				tagLogger.Debug("Tag matches, considering for deletion (-tag=", *tagRegexp, ")")
				if tag.Time.Before(deadline) {
					if ignoredTags < *latest {
						tagLogger.WithField("time", tag.Time).Infof("Ignore %d latest matching tags (-latest=%d)", *latest, *latest)
						ignoredTags += 1
					} else {
						tagLogger.WithField("tag", tag.Tag).WithField("time", tag.Time).Infof("Marking tag as outdated")
						markForDeletion = true
					}
				} else {
					tagLogger.Info("Tag not outdated")
					ignoredTags += 1
				}
			} else {
				tagLogger.Info("Ignore non matching tag (-tag=", *tagRegexp, ")")
			}

			if markForDeletion {
				deletableTags[tagIndex] = tag
			} else {
				nonDeletableTags[tagIndex] = tag
			}
		}

		// This approach is actually a workaround for the problem that Docker Distribution doesn't implement TagService.Untag operation at the time of this writing
		// Actually we have to delete the underlying image (specified via its Digest value), taking care not to delete images that are referenced by tags which we want to preserve
		nonDeletableDigests := make(map[string] string)
		for _, tag := range nonDeletableTags {
			if nonDeletableDigests[tag.Descriptor.Digest.String()] == "" {
				nonDeletableDigests[tag.Descriptor.Digest.String()] = tag.Tag
			} else {
				nonDeletableDigests[tag.Descriptor.Digest.String()] = nonDeletableDigests[tag.Descriptor.Digest.String()] + ", " + tag.Tag
			}
		}

		digestsDeleted := make(map[string] bool)
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
							logger.WithField("tag", tag.Tag).Error("Could not delete image!")
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
