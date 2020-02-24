FROM golang:1.13.5-alpine3.11 as build

RUN apk add --no-cache gcc libc-dev

ARG VERSION
ARG COMMIT

WORKDIR /workspace
ADD . /workspace/
RUN go build -ldflags "-X main.version=${VERSION:-dev} -X main.commit=${COMMIT:-none}" ./cmd/cloudbuild

FROM alpine:3.11.2

WORKDIR /workspace
COPY LICENSE /
COPY cloudbuildconfig.yaml /etc/cloudbuild/config.yaml
COPY --from=build /workspace/cloudbuild /

ENTRYPOINT ["/cloudbuild"]
