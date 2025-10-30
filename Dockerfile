FROM golang:1.25-alpine@sha256:aee43c3ccbf24fdffb7295693b6e33b21e01baec1b2a55acc351fde345e9ec34 AS build-image

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
RUN yum -y update openssl-libs ca-certificates krb5-libs
COPY --from=build-image /main /main
ENTRYPOINT [ "/main" ]
