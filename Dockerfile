FROM golang:1.13.5-alpine3.11 as build

RUN apk add --no-cache gcc libc-dev

WORKDIR /workspace
ADD . /workspace/
RUN go build ./cmd/cloudbuild

FROM alpine:3.11.2

WORKDIR /workspace
COPY LICENSE /
COPY cloudbuildconfig.yaml /etc/cloudbuild/config.yaml
COPY --from=build /workspace/cloudbuild /

ENTRYPOINT ["/cloudbuild"]
