FROM golang:1.15.6-alpine3.12 as dev

RUN apk add --no-cache gcc libc-dev
WORKDIR /workspace

FROM dev as build

ARG VERSION
ARG COMMIT

ADD . /workspace/
RUN go build -ldflags "-X main.version=${VERSION:-dev} -X main.commit=${COMMIT:-none}" ./cmd/cloudbuild

FROM alpine:3.12.3

WORKDIR /workspace
COPY LICENSE /
COPY cloudbuildconfig.yaml /etc/cloudbuild/config.yaml
COPY --from=build /workspace/cloudbuild /

ENTRYPOINT ["/cloudbuild"]
