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

#include <dm_master.h>

volatile bool server_running_flag = true;

pid_t master;

pid_t worker[ WORKER_NUM ];


// show DMFserver basic confgure
// dmf_server_show_info();


// register signal  kill -10 pid
static void signal_SIGUSR1(int signum) {

    if(signum == SIGUSR1) {
        server_running_flag = false;
        for(int i=0; i < WORKER_NUM; i++)
            kill(worker[i], SIGTERM);
    }
}


static void handle_signal() {
    signal(SIGUSR1, signal_SIGUSR1);
}


void master_daemonize() {
    handle_signal();
    printf("setting signal succeed!\n");
    printf("Now daemonize...\n");

    master = fork();

    if (master < 0) { 
        perror("fork"); exit(1); 
    } else if (master > 0) {

        // father exit
		printf("%d\n", master);
        exit(0);
    }
    
    printf("--daemon running  PID: %d\n", master);

    if (setsid() < 0) {
        perror("setsid"); exit(1);
    }

    umask(0);

    chdir("/");

    close(STDIN_FILENO);
    close(STDOUT_FILENO);
    close(STDERR_FILENO);

    open("/dev/null", O_RDWR);
    open("/dev/null", O_WRONLY);
    open("/dev/null", O_RDONLY);

}


extern void master_start_multi_process_server() {

	int serfd = create_socket( SERVER_DEFAULT_PORT );
// -------------------------------------------------------------
	pid_t pid;

    for(int i=0; i < WORKER_NUM; i++) {
        // start worker process
        pid = fork();
        if (pid == 0) {

			//  -----------------  start_server(serfd);

        } else if (pid < 0){
            perror("fork failed!");
        } else {
            // father process
            worker[i] = pid;
        }
    }

	master_check_and_restart(serfd);

}


static void master_check_and_restart(int serfd) {

    for(int i=0; i < WORKER_NUM; i++)
		printf("--worker nums:%d, pid:%d\n", i, worker[i]);
	printf("check pid : %d\n", getpid());
    
    int i, status;
    pid_t pid;

    while(server_running_flag) {

        for(i=0; i < WORKER_NUM; i++) {

            pid = waitpid(worker[i], &status, WNOHANG);
            if(pid == 0) {

                // worker is running

            } else if (pid == worker[i]) {
                // worker is down
                pid = fork();

                if (pid < 0) {
                    perror("fork failed!");
                    exit(1);
                } else if (pid == 0) {

					//   ------------  start_server(serfd);
                    
                } else {
                    // father
                    printf("[warn: ]Worker %d has been down, ", worker[i]);
                    worker[i] = pid;
                    printf("Start new worker %d \n", pid);
                }
            } else {
                perror("waitpid failed!");
                exit(1);
            }
        }
        sleep(1);
    }
}