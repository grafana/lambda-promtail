FROM golang:1.26-alpine@sha256:d4c4845f5d60c6a974c6000ce58ae079328d03ab7f721a0734277e69905473e5 AS build-image

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
