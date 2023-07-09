

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
#include <wait.h>     // for close

#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>
#include <stdatomic.h>
#include <sys/shm.h>
#include <string.h>
#define WORKER_NUM 2
pid_t worker[ WORKER_NUM ];


typedef struct shm_data_t {
    atomic_int is_readable;
    char buf[1024];
} shm_data_t ;

_Atomic int x;



void worker_function() {
    void* shm = NULL;
    shm_data_t* shared;
    char buffer[1024];

    int shm_fd;
    shm_fd = shmget((key_t)1324, sizeof(shm_data_t), 0666|IPC_CREAT);
    if(shm_fd == -1) exit(1);
    shm = shmat(shm_fd, 0, 0);
    if(shm == (void*)-1) exit(1);

    shared = (shm_data_t*)shm;
    while(1) {
        if(shared->is_readable == 1) {
            sleep(0.1);
        }
        if(shared->is_readable == 0) {
            printf("%d enter some text\n", getpid());
            fgets(buffer, 1024, stdin);
            strncpy(shared->buf, buffer, 1024);
            shared->is_readable = 1;
        }
    }
}


void create_process() {
	pid_t pid;

    for(int i=0; i < WORKER_NUM; i++) {
        // start worker process
        pid = fork();
        if (pid == 0) {

			worker_function();

        } else if (pid < 0){
            perror("fork failed!");
        } else {
            // father process
            worker[i] = pid;
        }
    }

}

int main() {
    void* shm = NULL;
    shm_data_t* shared;

    int shm_fd;
    shm_fd = shmget((key_t)1324, sizeof(shm_data_t), 0666|IPC_CREAT);
    if(shm_fd == -1) exit(1);
    shm = shmat(shm_fd, 0, 0);
    if(shm == (void*)-1) exit(1);

    shared = (shm_data_t*)shm;
    shared->is_readable = 0;


    create_process();


    while(1) {
        if(shared->is_readable == 1) {
            printf("You write %s\n", shared->buf);
            shared->is_readable = 0;
        }else{
            sleep(0.5);
        }
    }


    return 0;
}

