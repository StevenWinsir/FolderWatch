#ifndef FOLDERWATCH_FSEVENTS_H
#define FOLDERWATCH_FSEVENTS_H
#include <stdint.h>
typedef struct FWEventStream FWEventStream;
FWEventStream *fw_events_start(const char *root, uintptr_t handle);
void fw_events_close(FWEventStream *state);
#endif
