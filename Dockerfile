# Build the Go engine
FROM golang:1.24-alpine AS build
COPY gosnake /src/gosnake
WORKDIR /src/gosnake
RUN CGO_ENABLED=0 go build -trimpath -o /gosnake .

# Run it
FROM alpine:3.20
COPY --from=build /gosnake /gosnake
ENV PORT=8000
EXPOSE 8000
CMD ["/gosnake"]
