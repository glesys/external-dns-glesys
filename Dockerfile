FROM golang:1.24-alpine as base

FROM base as builder
# Work directory
WORKDIR /build

# Installing dependencies
COPY go.mod go.sum /build/

RUN go mod download

# Copying all the files
COPY . .

# Build our application
RUN go build -o /external-dns-glesys

FROM alpine:latest

COPY --from=builder --chown=root:root external-dns-glesys /bin/

# Drop to unprivileged user to run
USER nobody
CMD ["/bin/external-dns-glesys"]
