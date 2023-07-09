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

#ifndef __DM_THREADING_POLL_INCLUDE__
#define __DM_THREADING_POLL_INCLUDE__

#include <dm_server_config.h>
#include <dm_events.h>

#include <pthread.h>
#include <stdlib.h>

typedef struct {
    void* num1;
    int num2;
    int read_or_write;
} task_t;

typedef struct node_t {
    task_t task;
    struct node_t* next;
} node_t;

typedef struct {
    node_t* head;
    node_t* tail;
    pthread_mutex_t mutex;
    pthread_cond_t cond;
} queue_t;

typedef struct { 
    int num_threads; 
    pthread_t* threads; 
    queue_t* queues; 
    int stop; 
} thread_pool_t; 



#ifdef __cplusplus
extern "C" {
#endif


extern void     queue_init(queue_t* queue);
extern void     enqueue(queue_t* queue, task_t task);
extern task_t   dequeue(queue_t* queue);

extern void*    execute_task(void* arg);

extern void     thread_pool_init(thread_pool_t* threadpool, int num_threads);
extern void     thread_pool_destroy(thread_pool_t* threadpool);

extern void     add_task(thread_pool_t* threadpool, void* num1, int num2, int read_or_wite);



#ifdef __cplusplus
}		/* end of the 'extern "C"' block */
#endif


#endif  // __DM_THREADING_POLL_INCLUDE__

