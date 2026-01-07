FROM golang:1.25-alpine@sha256:ac09a5f469f307e5da71e766b0bd59c9c49ea460a528cc3e6686513d64a6f1fb AS build-image

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
