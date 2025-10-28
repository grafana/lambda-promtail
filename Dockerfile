FROM golang:1.24-alpine@sha256:8f8959f38530d159bf71d0b3eb0c547dc61e7959d8225d1599cf762477384923 AS build-image

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
FROM public.ecr.aws/lambda/provided:al2@sha256:2be50574d29388eda52f4e2ca0f9d9cd27240fcf465b220c642ff45d0e443894
RUN yum -y update openssl-libs ca-certificates krb5-libs
COPY --from=build-image /main /main
ENTRYPOINT [ "/main" ]
