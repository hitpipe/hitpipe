FROM docker.io/library/maven:3.9.11-eclipse-temurin-21@sha256:6fdc855a6ed81d288ca7ca37ac6ff5e9308b612485c0801d70b25a858c83d237 AS provider
WORKDIR /build
COPY pom.xml .
COPY src ./src
RUN --mount=type=cache,id=hit105-email-maven,target=/root/.m2/repository,sharing=locked \
    mvn --batch-mode --no-transfer-progress package

FROM quay.io/keycloak/keycloak:26.8.1@sha256:79c623a22abacf3fe6c00a5fb30f5b7843d717b234033b80995891c06116d244
COPY --from=provider --chown=1000:0 --chmod=0644 /build/target/hitpipe-keycloak-email-code.jar /opt/keycloak/providers/hitpipe-keycloak-email-code.jar
COPY --chown=1000:0 --chmod=0644 realm/hit105-test-realm.json /opt/keycloak/data/import/hit105-test-realm.json
RUN touch -m --date=@1794873600 /opt/keycloak/providers/hitpipe-keycloak-email-code.jar \
    && /opt/keycloak/bin/kc.sh build --db=postgres
