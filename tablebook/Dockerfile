FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/tablebook ./cmd/server
RUN mkdir /data

FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/tablebook /tablebook
COPY --from=build --chown=65532:65532 /data /data
ENV DB_PATH=/data/tablebook.db
EXPOSE 8080
VOLUME /data
ENTRYPOINT ["/tablebook"]
