FROM golang:alpine AS builder
ARG VERSION
# install certificates to copy it to the scratch container later
# needed to let the validation of TLS certs work
RUN apk update && apk add --no-cache ca-certificates git

WORKDIR /build
COPY . .
# 
RUN sh ./build.sh

# try to start it to let the build fail in case of emergency
RUN /build/go-myapps-app-opentalk -h

FROM scratch 
ARG VERSION
WORKDIR /
COPY --from=builder /build/go-myapps-app-opentalk /go-myapps-app-opentalk
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
LABEL \
  org.opencontainers.image.vendor="Rico Schulte" \
  org.opencontainers.image.title="go-myapps-app-opentalk" \
  org.opencontainers.image.version="$VERSION" \
  org.opencontainers.image.source="https://github.com/ricoschulte/go-myapps-app-opentalk"
EXPOSE 3000 3001
VOLUME ["/data"]
ENTRYPOINT ["/go-myapps-app-opentalk"]
