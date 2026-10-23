#include <assert.h>
#include "../drm_native.c"

static uint32_t crtc_ids[] = {10, 11};
static uint32_t connector_ids[] = {20, 21, 22};
static OnixDRMResources resources = {.count_crtcs = 2, .crtcs = crtc_ids, .count_connectors = 3, .connectors = connector_ids};
static int restored, freed_crtcs, fail_crtc;

static OnixDRMResources *get_resources(int fd) {
    (void)fd;
    return &resources;
}

static void free_resources(OnixDRMResources *value) { assert(value == &resources); }

static OnixDRMCrtc *get_crtc(int fd, uint32_t id) {
    (void)fd;
    if (fail_crtc && id == 11) return NULL;
    OnixDRMCrtc *crtc = calloc(1, sizeof(*crtc));
    assert(crtc != NULL);
    crtc->crtc_id = id;
    if (id == 10) {
        crtc->mode_valid = 1;
        crtc->buffer_id = 30;
        crtc->x = 40;
        crtc->mode.hdisplay = 1920;
    }
    return crtc;
}

static void free_crtc(OnixDRMCrtc *crtc) {
    freed_crtcs++;
    free(crtc);
}

static OnixDRMConnector *get_connector(int fd, uint32_t id) {
    (void)fd;
    OnixDRMConnector *connector = calloc(1, sizeof(*connector));
    assert(connector != NULL);
    connector->connector_id = id;
    connector->encoder_id = id == 22 ? 0 : 100 + id;
    return connector;
}

static void free_connector(OnixDRMConnector *connector) { free(connector); }

static OnixDRMEncoder *get_encoder(int fd, uint32_t id) {
    (void)fd;
    OnixDRMEncoder *encoder = calloc(1, sizeof(*encoder));
    assert(encoder != NULL);
    encoder->encoder_id = id;
    encoder->crtc_id = 10;
    return encoder;
}

static void free_encoder(OnixDRMEncoder *encoder) { free(encoder); }

static int set_crtc(int fd, uint32_t id, uint32_t buffer, uint32_t x, uint32_t y, uint32_t *connectors, int count, OnixDRMModeInfo *mode) {
    assert(fcntl(fd, F_GETFD) >= 0);
    assert(y == 0);
    if (restored == 0) {
        assert(id == 11 && buffer == 0 && count == 0 && mode == NULL);
    } else {
        assert(restored == 1 && id == 10 && buffer == 30 && x == 40);
        assert(count == 2 && connectors[0] == 20 && connectors[1] == 21);
        assert(mode->hdisplay == 1920);
    }
    restored++;
    return 0;
}

static OnixDRMDisplay *new_display(void) {
    OnixDRMDisplay *display = calloc(1, sizeof(*display));
    assert(display != NULL);
    display->fd = open("/dev/null", O_RDWR | O_CLOEXEC);
    assert(display->fd >= 0);
    display->get_resources = get_resources;
    display->free_resources = free_resources;
    display->get_crtc = get_crtc;
    display->free_crtc = free_crtc;
    display->get_connector = get_connector;
    display->free_connector = free_connector;
    display->get_encoder = get_encoder;
    display->free_encoder = free_encoder;
    display->set_crtc = set_crtc;
    restored = freed_crtcs = fail_crtc = 0;
    return display;
}

int main(void) {
    OnixDRMDisplay *display = new_display();
    display->mode = (OnixDRMModeInfo){.clock = 148500, .htotal = 2200, .vtotal = 1125};
    assert(onix_drm_refresh_rate(display) == 60000);
    display->mode.clock = 148352;
    assert(onix_drm_refresh_rate(display) == 59940);
    display->mode.clock = 74250;
    display->mode.flags = 1u << 4;
    assert(onix_drm_refresh_rate(display) == 60000);
    display->mode.flags = 1u << 5;
    assert(onix_drm_refresh_rate(display) == 15000);
    display->mode.vscan = 2;
    assert(onix_drm_refresh_rate(display) == 7500);
    assert(onix_drm_save_outputs(display));
    assert(display->gbm_library == NULL && display->gbm == NULL && display->surface == NULL);
    onix_drm_restore_on_destroy(display);
    int fd = display->fd;
    onix_drm_destroy(display);
    assert(restored == 2 && freed_crtcs == 2);
    assert(fcntl(fd, F_GETFD) == -1 && errno == EBADF);

    display = new_display();
    assert(onix_drm_save_outputs(display));
    onix_drm_destroy(display);
    assert(restored == 0 && freed_crtcs == 2);

    display = new_display();
    fail_crtc = 1;
    assert(!onix_drm_save_outputs(display));
    onix_drm_destroy(display);
    assert(restored == 0 && freed_crtcs == 1);
    return 0;
}
