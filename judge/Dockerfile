FROM rust:1.83-alpine AS build
RUN apk add --no-cache musl-dev
WORKDIR /src
COPY Cargo.toml ./
COPY src ./src
RUN cargo build --release
FROM docker:27-cli
RUN apk add --no-cache libgcc
COPY --from=build /src/target/release/avernaarena-judge /usr/local/bin/avernaarena-judge
EXPOSE 8090
ENTRYPOINT ["/bin/sh", "-c", "docker pull \"${RUNNER_IMAGE:-python:3.12-alpine}\" >/dev/null && exec avernaarena-judge"]
