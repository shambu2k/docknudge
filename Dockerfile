FROM golang:1.25 AS build
WORKDIR /src

COPY go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/docknudge ./cmd/docknudge

FROM gcr.io/distroless/static-debian12
WORKDIR /app
COPY --from=build /out/docknudge /usr/local/bin/docknudge

ENTRYPOINT ["/usr/local/bin/docknudge"]
CMD ["run"]
