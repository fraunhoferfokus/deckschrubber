FROM centurylink/ca-certs
ADD main /
ENTRYPOINT ["/main"]