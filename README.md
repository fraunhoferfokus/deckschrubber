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
-latest int
      number of the latest matching images of an repository that won't be deleted (default 1)      
-month int
      max age in months
-registry string
      URL of registry (default "http://localhost:5000")
-repo string
      matching repositories (allows regexp) (default ".*")      
-repos int
      number of repositories to garbage collect (default 5)
-tag string
      matching tags (allows regexp) (default ".*")      
-v    shows version and quits
-year int
      max age in years
      
      
```

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
