FROM --platform=$BUILDPLATFORM golang:1.22 AS build
ARG TARGETARCH
WORKDIR /usr/src/app
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/gomod-cache GOMODCACHE=/gomod-cache go mod download
COPY *.go ./
COPY internal internal
RUN --mount=type=cache,target=/go-cache --mount=type=cache,target=/gomod-cache GOCACHE=/go-cache GOMODCACHE=/gomod-cache CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build -tags netgo -ldflags -w -o bin/calico-auto-policy-$TARGETARCH calico-auto-policy.go

FROM alpine:latest
ARG TARGETARCH
COPY --from=build /usr/src/app/bin/calico-auto-policy-$TARGETARCH /usr/local/bin/calico-auto-policy
CMD ["calico-auto-policy"]
