FROM golang:1.25-alpine@sha256:e6898559d553d81b245eb8eadafcb3ca38ef320a9e26674df59d4f07a4fd0b07 AS build-image

COPY . /src/lambda-promtail
WORKDIR /src/lambda-promtail

RUN go version

RUN apk update && apk upgrade && \
    apk add --no-cache bash git
RUN go version

RUN ls -al
RUN go mod download
RUN go build -o /main -tags lambda.norpc -ldflags="-s -w" pkg/*.go
# copy artifacts to a clean image
FROM public.ecr.aws/lambda/provided:al2@sha256:5237e09330b1b06b9f5f7eb2cbd8bd8b091ac4a7e3a9f82d679bd2423e063b35
RUN yum -y update glib2 stdlib openssl-libs ca-certificates krb5-libs && \
    yum clean all && \
    rm -rf /var/cache/yum
COPY --from=build-image /main /main
ENTRYPOINT [ "/main" ]
