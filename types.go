package main

import (
	"time"

	"github.com/docker/distribution"
)

// BlobInfo represents information about a specific Blob
type BlobInfo struct {
	Created time.Time `json:"created"` // Creation time
}

// Image represents a docker image with a specific tag
type Image struct {
	Repository string                  // Name of repository to which image belongs
	Tag        string                  // Image's tag
	Time       time.Time               // Creation time of the image
	Descriptor distribution.Descriptor // Underlying image descriptor
}

// ImageByDate represents an array of images
// sorted by creation date
type ImageByDate []Image

func (ibd ImageByDate) Len() int           { return len(ibd) }
func (ibd ImageByDate) Swap(i, j int)      { ibd[i], ibd[j] = ibd[j], ibd[i] }
func (ibd ImageByDate) Less(i, j int) bool { return ibd[i].Time.Before(ibd[j].Time) }
