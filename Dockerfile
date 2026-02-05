FROM golang:1.25-alpine@sha256:98e6cffc31ccc44c7c15d83df1d69891efee8115a5bb7ede2bf30a38af3e3c92 AS build-image

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
FROM public.ecr.aws/lambda/provided:al2.2025.12.22.12@sha256:5191eb43a2bc33971e3f8bf86eca599b47850d45e891523c909389153419f891
RUN yum -y update glib2 stdlib openssl-libs ca-certificates krb5-libs && \
    yum clean all && \
    rm -rf /var/cache/yum
COPY --from=build-image /main /main
ENTRYPOINT [ "/main" ]
