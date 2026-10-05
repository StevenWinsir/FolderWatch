//go:build darwin && cgo

#include "fsevents_darwin.h"
#include <CoreServices/CoreServices.h>
#include <dispatch/dispatch.h>
#include <stdlib.h>

extern void folderwatchEvent(uintptr_t handle, char *path, uint32_t flags);

struct FWEventStream {
    FSEventStreamRef stream;
    dispatch_queue_t queue;
};

static void callback(ConstFSEventStreamRef stream, void *context, size_t count,
                     void *paths, const FSEventStreamEventFlags flags[],
                     const FSEventStreamEventId ids[]) {
    char **names = (char **)paths;
    for (size_t i = 0; i < count; i++) {
        folderwatchEvent((uintptr_t)context, names[i], flags[i]);
    }
}

static void barrier(void *unused) {}

void fw_events_close(FWEventStream *state) {
    if (!state) return;
    if (state->stream) {
        FSEventStreamStop(state->stream);
        FSEventStreamInvalidate(state->stream);
        // Join any callback already admitted to the serial queue before the
        // Go cgo.Handle can be deleted. Close never runs on this queue.
        if (state->queue) dispatch_sync_f(state->queue, NULL, barrier);
        FSEventStreamRelease(state->stream);
    }
    if (state->queue) dispatch_release(state->queue);
    free(state);
}

FWEventStream *fw_events_start(const char *root, uintptr_t handle) {
    CFStringRef path = CFStringCreateWithFileSystemRepresentation(kCFAllocatorDefault, root);
    if (!path) return NULL;
    CFArrayRef paths = CFArrayCreate(kCFAllocatorDefault, (const void **)&path, 1, &kCFTypeArrayCallBacks);
    CFRelease(path);
    if (!paths) return NULL;
    FWEventStream *state = calloc(1, sizeof(*state));
    if (!state) { CFRelease(paths); return NULL; }
    FSEventStreamContext context = {0, (void *)handle, NULL, NULL, NULL};
    state->stream = FSEventStreamCreate(kCFAllocatorDefault, callback, &context, paths,
        kFSEventStreamEventIdSinceNow, 0.05,
        kFSEventStreamCreateFlagNoDefer | kFSEventStreamCreateFlagWatchRoot |
        kFSEventStreamCreateFlagFileEvents);
    CFRelease(paths);
    if (!state->stream) { fw_events_close(state); return NULL; }
    state->queue = dispatch_queue_create("FolderWatch.FSEvents", DISPATCH_QUEUE_SERIAL);
    if (!state->queue) { fw_events_close(state); return NULL; }
    FSEventStreamSetDispatchQueue(state->stream, state->queue);
    if (!FSEventStreamStart(state->stream)) { fw_events_close(state); return NULL; }
    return state;
}
