FROM golang:1.24.3 AS build-stage

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY *.go ./

RUN CGO_ENABLED=0 GOOS=linux go build -o /bsky-sampler
FROM gcr.io/distroless/base-debian11 AS build-release-stage

WORKDIR /
COPY --from=build-stage /bsky-sampler /bsky-sampler
EXPOSE 80
USER nonroot:nonroot
ENTRYPOINT ["/bsky-sampler"]