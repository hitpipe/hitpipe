package dev.hitpipe.keycloak.email;

import java.util.List;
import org.keycloak.Config;
import org.keycloak.connections.jpa.entityprovider.JpaEntityProvider;
import org.keycloak.connections.jpa.entityprovider.JpaEntityProviderFactory;
import org.keycloak.models.KeycloakSession;
import org.keycloak.models.KeycloakSessionFactory;

public final class EmailCodeJpaProviderFactory implements JpaEntityProviderFactory {
    private static final String ID = "hitpipe-email-code-schema";

    @Override
    public JpaEntityProvider create(KeycloakSession session) {
        return new JpaEntityProvider() {
            @Override
            @SuppressWarnings("removal")
            public List<Class<?>> getEntities() {
                return List.of();
            }

            @Override
            public String getChangelogLocation() {
                return "META-INF/hitpipe-email-code-changelog.xml";
            }

            @Override
            public String getFactoryId() {
                return ID;
            }

            @Override
            public void close() {}
        };
    }

    @Override
    public String getId() {
        return ID;
    }

    @Override
    public void init(Config.Scope config) {}

    @Override
    public void postInit(KeycloakSessionFactory factory) {}

    @Override
    public void close() {}
}
