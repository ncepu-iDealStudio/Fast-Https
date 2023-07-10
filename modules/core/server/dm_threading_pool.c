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

#include <dm_threading_pool.h>


void queue_init(queue_t* queue) { 
    queue->head = NULL; 
    queue->tail = NULL; 
    pthread_mutex_init(&(queue->mutex), NULL); 
    pthread_cond_init(&(queue->cond), NULL); 
}

void enqueue(queue_t* queue, task_t task) { 
    node_t* newNode = (node_t*)malloc(sizeof(node_t)); 
    newNode->task = task; 
    newNode->next = NULL;
    
    pthread_mutex_lock(&(queue->mutex)); 
    if (queue->head == NULL) { 
        queue->head = newNode; 
        queue->tail = newNode; 
        pthread_cond_signal(&(queue->cond)); // 唤醒等待的线程 
	} else { 
		queue->tail->next = newNode; queue->tail = newNode; 
	} 
    pthread_mutex_unlock(&(queue->mutex)); 
} 

task_t dequeue(queue_t* queue) { 
    task_t task; 
    pthread_mutex_lock(&(queue->mutex)); 
    while (queue->head == NULL) { 
        pthread_cond_wait(&(queue->cond), &(queue->mutex)); // 等待队列非空 
    }
    node_t* temp = queue->head; 
    task = temp->task; 
    queue->head = queue->head->next; 
    free(temp); 
    if (queue->head == NULL) { 
        queue->tail = NULL; 
    } 
    pthread_mutex_unlock(&(queue->mutex)); 
    return task; 
}


void* execute_task(void* arg) {
    queue_t* queue = (queue_t*)arg;
    task_t task;
    for (;;) {
        task = dequeue(queue);
        if (task.num1 == NULL && task.num2 == -1) {
            break;
        }
        switch (task.read_or_write) {
		case 2:
			//handle_read (task.num1, task.num2);
			break;
		case 3:
			//handle_write (task.num1, task.num2);
			break;
		default:
			break;
		}
	}
    return NULL;
}

void thread_pool_init(thread_pool_t* threadPool, int num_threads) {
    threadPool->num_threads = num_threads;
    threadPool->threads = (pthread_t*)malloc(num_threads * sizeof(pthread_t));
    threadPool->queues = (queue_t*)malloc(num_threads * sizeof(queue_t));
    threadPool->stop = 0;
    int i;
    for (i = 0; i < num_threads; i++) {
        queue_init(&(threadPool->queues[i]));
        pthread_create(&(threadPool->threads[i]), NULL, execute_task, &(threadPool->queues[i]));
    }
}



void thread_pool_destroy(thread_pool_t* threadPool) { 
    int i; 
    threadPool->stop = 1; // 发送停止信号 
    for (i = 0; i < threadPool->num_threads; i++) { 
        enqueue(&(threadPool->queues[i]), (task_t){NULL, -1}); // 发送退出信号 
        pthread_cond_signal(&(threadPool->queues[i].cond)); // 唤醒等待的线程 
        pthread_join(threadPool->threads[i], NULL); 
        pthread_mutex_destroy(&(threadPool->queues[i].mutex)); 
        pthread_cond_destroy(&(threadPool->queues[i].cond)); 
    } 
    free(threadPool->threads); 
    free(threadPool->queues); 
}


void add_task(thread_pool_t* threadPool, void* num1, int num2, int read_or_wite) { 
    task_t task; 
    task.num1 = num1; 
    task.num2 = num2; 
    task.read_or_write = read_or_wite;
    
    int index = rand() % threadPool->num_threads; 
	// printf("%d\n", index);
    enqueue(&(threadPool->queues[0]), task); 
} 