package dev.hitpipe.keycloak.email;

import org.keycloak.Config;
import org.keycloak.authentication.RequiredActionProvider;
import org.keycloak.events.Event;
import org.keycloak.events.EventListenerProvider;
import org.keycloak.events.EventListenerProviderFactory;
import org.keycloak.events.EventType;
import org.keycloak.events.admin.AdminEvent;
import org.keycloak.events.admin.OperationType;
import org.keycloak.events.admin.ResourceType;
import org.keycloak.models.KeycloakSession;
import org.keycloak.models.KeycloakSessionFactory;

public final class EmailCodeAccountEvents implements EventListenerProviderFactory {
    private static final String ID = "hitpipe-email-code-account-events";

    @Override
    public EventListenerProvider create(KeycloakSession session) {
        return new EventListenerProvider() {
            @Override
            public void onEvent(Event event) {
                EventType type = event.getType();
                if (event.getError() == null && (type == EventType.UPDATE_EMAIL || type == EventType.UPDATE_PROFILE)) {
                    revokeIfEmailChanged(session, event.getRealmId(), event.getUserId());
                }
            }

            @Override
            public void onEvent(AdminEvent event, boolean includeRepresentation) {
                String path = event.getResourcePath();
                if (event.getError() == null && event.getResourceType() == ResourceType.USER
                        && event.getOperationType() == OperationType.UPDATE && path != null && path.matches("users/[^/]+")) {
                    revokeIfEmailChanged(session, event.getRealmId(), path.substring("users/".length()));
                }
            }

            @Override
            public void close() {}
        };
    }

    private static void revokeIfEmailChanged(KeycloakSession session, String realmId, String userId) {
        var factory = session.getKeycloakSessionFactory()
                .getProviderFactory(RequiredActionProvider.class, EmailCodeActionFactory.ID);
        if (factory instanceof EmailCodeActionFactory action && action.store() != null && userId != null) {
            action.store().revokeOnEmailChange(session, realmId, userId);
        }
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
