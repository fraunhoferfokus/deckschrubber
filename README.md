# Deckschrubber
> *n. person responsible for scrubbing a ship's deck.*

[![Go Report Card](https://goreportcard.com/badge/github.com/fraunhoferfokus/deckschrubber)](https://goreportcard.com/report/github.com/fraunhoferfokus/deckschrubber)
[![License](https://img.shields.io/github/license/fraunhoferfokus/sesame.svg)](https://github.com/fraunhoferfokus/sesame/blob/master/LICENSE)

Deckschrubber inspects images of a [Docker Registry](https://docs.docker.com/registry/) and removes those older than a given age.

## Quick Start

```bash
# Use GOLANG
go get github.com/fraunhoferfokus/deckschrubber
$GOPATH/bin/deckschrubber
```

## Why this?
We run our own private registry on a server with limited storage and it was only a question of time, until it was full! Although there are similar approaches to do Docker registry house keeping (in Python, Ruby, etc), a native module (using Docker's own packages) was missing. This module has the following advantages:

* **Is binary!**: No runtime, Python, Ruby, etc. is required
* **Uses Docker API**: same packages and modules used to relaze Docker registry are used here

## Arguments
```
-v    shows version and quits
-debug
      run in debug mode
-dry
      does not actually deletes
-registry string
      URL of registry (default "http://localhost:5000")
-username string
      username for docker login
-password string
      password for docker login
-insecure
      ignore https verification error
-repos
      matching repositories by name (allows mulitple value seperates by ,); If specified will ignore `-repo_regexp`
-repo_regexp string
      matching repositories (allows regexp) (default ".*")
-max_repos int
      max nubmer of repositories to garbage collect (default to no limit)
-tag_regexp string
      matching tags (allows regexp) (default ".*")
-latest int
      number of the latest matching images of an repository that won't be deleted (default 1)
-year int
      max age in years
-month int
      max age in months
-day int
      max age in days
      
```

## Examples

* **Remove all images older than 2 months and 2 days**

```
$GOPATH/bin/deckschrubber -month 2 -day 2
```

* **Remove these images older than 1 year from `http://myrepo:5000`**

```
$GOPATH/bin/deckschrubber -year 1 -registry http://myrepo:5000 -repos myproject/myimage,myproject/otherimage -username myself -password mypwd
```

* **Remove all images of each repository except the 3 latest ones**

```
$GOPATH/bin/deckschrubber -latest 3 
```

* **Remove all images with tags that ends with '-SNAPSHOT'**

```
$GOPATH/bin/deckschrubber -tag_regexp ^.*-SNAPSHOT$ 
```

## Dockerize

In order to have a minimum image footprint(~7+MB), the dockerize process had avoid to use the offical [golang image](https://hub.docker.com/_/golang/).
But to compile golang app alone and build the image from a minimum image that only container a CA certificate [centurylink/ca-certs](https://hub.docker.com/r/centurylink/ca-certs/). 
Please follow these steps to have a working image built and pushed:


* **Compile golang app with the following command**

```
CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main .
```

* **Build and push the image with proper tag**

```
docker build -t your.registry.com:5000/someproject/deckschrubber:20171025-3-SNAPSHOT .
docker push your.registry.com:5000/someproject/deckschrubber:20171025-3-SNAPSHOT 
```

* **Run deckschrubber as image**

```
docker run --rm --name registry-retention-runner  your.registry.com:5000/someproject/deckschrubber -registry http://your.registry.com:5000 -repos developer/myapp,developer/deckschrubber -username someone -password urpwd -insecure
```
