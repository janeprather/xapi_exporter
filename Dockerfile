FROM golang:latest

ADD . /go/src/github.com/janeprather/xapi_exporter

RUN go get github.com/janeprather/xapi_exporter

EXPOSE 9290

VOLUME ["/xapi_exporter"]

CMD /go/bin/xapi_exporter -config /xapi_exporter/config.yml
