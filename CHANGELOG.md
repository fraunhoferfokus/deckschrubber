# CHANGELOG
* `0.7.0`:
  * Fix logrus issue (name case)
  * Strucured as Go module
  * Add compact logged list of fetched repos
  * Friendlier API (usage on no args!)
* `0.6.0`: Add basic authentication
* `0.5.0`: Add `ntag` flag to match "everything but this" tags
* `0.4.0`:
  * Corrected behavior with images that have more than one tag (bug #9)
  * Changed the meaning of time-limit (e.g `-day`) in combination with `-latest` flag: it only takes into account whichever means more preserved matching tags
* `0.3.0`: Adapt to new Docker Distribution API
* `0.2.0`:
  * Null pointer bug fix
  * Additional features (match by tag/repo, custom latest ignore, debug mode)
* `0.1.0`: First working draf
  * Works only with `http`
  * Number of repositories can be limited
  * Age in years, months, days, or a combination
  * Allows dry run