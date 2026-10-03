FROM node:22-alpine AS frontend-build
WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM golang:1.23-alpine AS server-build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /lunarfall .

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=server-build /lunarfall ./lunarfall
COPY --from=frontend-build /app/frontend/dist ./frontend/dist
ENV PORT=8080
EXPOSE 8080
ENTRYPOINT ["/app/lunarfall"]