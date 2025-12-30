# Deckschrubber
> *n. person responsible for scrubbing a ship's deck.*

[![Go Report Card](https://goreportcard.com/badge/github.com/fraunhoferfokus/deckschrubber)](https://goreportcard.com/report/github.com/fraunhoferfokus/deckschrubber)
[![License](https://img.shields.io/github/license/fraunhoferfokus/sesame.svg)](https://github.com/fraunhoferfokus/sesame/blob/master/LICENSE)

Deckschrubber inspects images of a [Docker Registry](https://docs.docker.com/registry/) and removes those older than a given age.

## Quick Start

```bash
go get github.com/fraunhoferfokus/deckschrubber
$GOPATH/bin/deckschrubber
```

## Why this?
We run our own private registry on a server with limited storage and it was only a question of time, until it was full! Although there are similar approaches to do Docker registry house keeping (in Python, Ruby, etc), a native module (using Docker's own packages) was missing. This module has the following advantages:

* **Is binary!**: No runtime, Python, Ruby, etc. is required
* **Uses Docker API**: same packages and modules used to relaze Docker registry are used here

## Arguments
```
-day int
  max age in days
-debug
  run in debug mode
-dry
  does not actually deletes
-insecure
  Skip insecure TLS verification
-latest int
  number of the latest matching images of an repository that won't be deleted (default 1)
-month int
  max age in months
-ntag string
  non matching tags (allows regexp)
-page-size int
  Number of entries to fetch upon each request (default = 100) (default 100)
-paginate
  Set to use pagination when fetching repositories (default = false)
-password string
  Password for basic authentication
-registry string
  URL of registry (default "http://localhost:5000")
-repo string
  matching repositories (allows regexp) (default ".*")
-repos int
  number of repositories to garbage collect (before filtering, lexographically sorted by server) (default 5)
-tag string
  matching tags (allows regexp) (default ".*")
-user string
  Username for basic authentication
-v    shows version and quits
-year int
  max age in days
```

## Proxy
To access the target registry via proxy, set proxy environment variable(s) as described by Golang's [net/http package](https://pkg.go.dev/net/http#ProxyFromEnvironment).

## Registry preparation
Deckschrubber uses the Docker Registry API. 
Its delete endpoint is disabled by default, you have to enable it with the following entry in the registry configuration file: 

```
delete:
  enabled: true
```

See [the documentation](https://distribution.github.io/distribution/about/configuration/#delete) for details.

## Examples

* **Remove all images older than 2 months and 2 days**

```
$GOPATH/bin/deckschrubber -month 2 -day 2
```

* **Remove all images older than 1 year from `http://myrepo:5000`**

```
$GOPATH/bin/deckschrubber -year 1 -registry http://myrepo:5000
```

* **Analyize (but do not remove) images of 30 repositories**

```
$GOPATH/bin/deckschrubber -repos 30 -dry
```

* **Remove all images of each repository except the 3 latest ones**

```
$GOPATH/bin/deckschrubber -latest 3 
```

* **Remove all images with tags that ends with '-SNAPSHOT'**

```
$GOPATH/bin/deckschrubber -tag ^.*-SNAPSHOT$ 
```

* **Use pagination when querying server**

```
$GOPATH/bin/deckschrubber -paginate -page-size 50 -repos 150
```

*Note* that `-paginate` must be present for `-page-size` to have an effect. If pagination is enabled, `-repos` should be larger than `-page-size`, otherwise it has the same effect as without pagination.