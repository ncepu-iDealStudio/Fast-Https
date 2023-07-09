/* 
    *  Copyright 2023 Ajax
    *
    *  Licensed under the Apache License, Version 2.0 (the "License");
    *  you may not use this file except in compliance with the License.
    *
    *  You may obtain a copy of the License at
    *
    *    http://www.apache.org/licenses/LICENSE-2.0
    *    
    *  Unless required by applicable law or agreed to in writing, software
    *  distributed under the License is distributed on an "AS IS" BASIS,
    *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
    *  See the License for the specific language governing permissions and
    *  limitations under the License. 
    *
    */

#ifndef __DM_TIMER_INCLUDE__
#define __DM_TIMER_INCLUDE__

#include <stdio.h>
#include <stdlib.h>
#include <time.h>
#include <stdbool.h>
#include <unistd.h>

#define TIMER_EVENT_MAX_NUM 10240

typedef struct timer_event_t {
    time_t timeout;
    void (*callback)();
} timer_event_t;

typedef struct timer_min_heap_t {
    timer_event_t *events[10240];
    int size;
} timer_min_heap_t;

#ifdef __cplusplus
extern "C" {
#endif

    static void             min_heap_push(timer_min_heap_t *heap, timer_event_t *event);
    static timer_event_t*   min_heap_top(timer_min_heap_t *heap);
    static void             min_heap_pop(timer_min_heap_t *heap);
    void                    handle_events(timer_min_heap_t *heap);
    void                    add_timer(timer_min_heap_t *heap, int timeout, void (*callback)());
    extern int              test_timer();

#ifdef __cplusplus
}           /* end of the 'extern "C"' block */
#endif


#endif // __DM_TIMER_INCLUDE__