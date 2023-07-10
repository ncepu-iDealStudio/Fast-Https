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
   
#include <sys/epoll.h>
#include <unistd.h>     // for close
#include <stdio.h>
#include <string.h>
#include <fcntl.h>
#include <sys/socket.h>
#include <netinet/in.h>
#include <pthread.h>
#include <stdlib.h>
#include <errno.h>
#include <stdbool.h>
#include <sys/time.h>

#include <signal.h>		// signal()
#include <sys/wait.h>   // waitpid()
#include <sys/stat.h>   // umask(0)

#define MAX_EVENT_NUM 1024

#define WORKER_NUM 40

#define SERVER_PORT 8080

static pid_t master;

static pid_t worker[ WORKER_NUM ];

volatile bool server_running_flag = true;

static char send_buf[] = "HTTP/1.1 200 OK\r\n\r\nhello"
	"lajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdj"
	"lajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdj"
	"lajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdj"
	"lajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdj"
	"lajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdj"
	"lajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdj"
	"lajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdj"
	"lajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdj"
	"lajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdj"
	"lajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdj"
	"lajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdj"
	"lajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdj"
	"lajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdj"
	"lajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdj"
	"lajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdj"
	"lajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdj"
	"lajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdj"
	"lajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdj"
	"lajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdj"
	"lajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdj"
	"lajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdj"
	"lajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdjlajsdf;ljlasjdlfjl;ajslkjf;lkasdj";


int total_accept_num = 0;




static int   set_non_blocking(int);
static int   set_reuse(int);


static void  handle_accept(int, int);
static void  handle_read(int, int);
static void  handle_write(int, int);



static int   create_socket(int);
static void* server_make();
static void  daemonize();
static void  check_and_restart(int);
static void  signal_handle(int) ;
static void  dmf_server_show_info();


int set_non_blocking(int fd) {

	int flag = fcntl(fd, F_GETFL, 0);
	if(flag == -1) 
		return -1;
	
	flag |= O_NONBLOCK;
	if(fcntl(fd, F_SETFL, flag) == -1)
		return -1;
	return 0;
}

int set_reuse(int i_listenfd) {
	int out = 2;
    return setsockopt(i_listenfd, SOL_SOCKET, SO_REUSEADDR, &out, sizeof(out));
}


void handle_accept (int serfd, int epoll_fd) {
	struct sockaddr_in cliaddr;
	int socklen = sizeof(cliaddr);
	struct epoll_event ev;
	int clifd;

	while( (clifd = accept(serfd, (struct sockaddr*)&cliaddr, &socklen)) > 0) {
	
		if(set_non_blocking(clifd) == -1) {
			perror("set_non_blocking2");
			return;
		}

		// epoll_ctl(epoll_fd, EPOLL_CTL_DEL, clifd, NULL);
		// close(clifd);
		
		ev.events = EPOLLIN | EPOLLET | EPOLLONESHOT;
		ev.data.fd = clifd;
		if( epoll_ctl(epoll_fd, EPOLL_CTL_ADD, clifd, &ev) == -1) {
			perror("epoll_ctl add");
			close(clifd);
		}	
	}
	if(clifd == -1) {
		if(errno != EAGAIN )
		printf("accept %s\n", strerror(errno));
		return;
	}
	// printf("%d %d\n", clifd, total_accept_num ++);
}

void handle_read (int client_fd, int epoll_fd) {
	char buf[512] = {0};
	ssize_t total_read = 0;
	ssize_t bytes_read;
	struct epoll_event ev;

	while(1) {
		bytes_read = read(client_fd, buf + total_read, 512 - total_read);
		if(bytes_read == -1) {
			if(errno == EAGAIN || errno == EWOULDBLOCK) {
				break;
			}else{
				perror("read error");
				//if( epoll_ctl(epoll_fd, EPOLL_CTL_DEL, client_fd, NULL) == -1)
				//perror("epoll_ctl error");
				//close(client_fd);
				return;
			}
		}else if(bytes_read == 0) {
			if( epoll_ctl(epoll_fd, EPOLL_CTL_DEL, client_fd, NULL) != -1) {
				close(client_fd);
			} else {
				perror("epoll del error");
			}
			return;
		}
		total_read += bytes_read;
	}
	// if(total_read != 123)
	// 	printf("%ld\n", total_read);

	ev.events = EPOLLOUT | EPOLLET ;
	ev.data.fd = client_fd;
	epoll_ctl(epoll_fd, EPOLL_CTL_MOD, client_fd, &ev);
	// handle_write(client_fd, epoll_fd);
	// sleep(0.1);
}

void handle_write (int client_fd, int epoll_fd) {
	ssize_t n, nwrite;
	ssize_t data_size = strlen(send_buf);
	n = data_size;

	while(n > 0){
		nwrite = write(client_fd, send_buf + data_size -n, n);
		if(nwrite < n) {
			if(nwrite == -1 && errno != EAGAIN) {
				// perror("write error");
				// if( epoll_ctl(epoll_fd, EPOLL_CTL_DEL, client_fd, NULL) != -1) {
				// 	close(client_fd);
				// } else {
				// 	perror("epoll_ctl error");
				// }
				return;
			}
			break;
		}
		n -= nwrite;
	}
	
	if( epoll_ctl(epoll_fd, EPOLL_CTL_DEL, client_fd, NULL) != -1) {
		close(client_fd);
	} else {
		perror("epoll del error");
	}
	// sleep(0.05);
}


int create_socket(int port) {

	int serfd;
	serfd = socket(AF_INET, SOCK_STREAM, 0);
	if(serfd == -1) {
		perror("socket error");
	}
	
	struct sockaddr_in addr;
	addr.sin_family = AF_INET;
	addr.sin_addr.s_addr = INADDR_ANY;
	addr.sin_port = htons(port);

	if(set_reuse(serfd) == -1) {
		perror("set_reuse error");
	}
	if( set_non_blocking(serfd) == -1){
		perror("set_non_blocking error");
	}
	if( bind(serfd, (struct sockaddr*)&addr, sizeof(addr)) == -1) {
		perror("bind error");
	}
	if( listen(serfd, 5) == -1) {
		perror("listen error");
	}
	struct timeval tv = {0, 500};
    setsockopt(serfd, SOL_SOCKET, SO_RCVTIMEO, &tv, sizeof(struct timeval));
	
	return serfd;
}

struct arg_t {
	int serfd;
};

void* server_make(void* arg) {
	
	struct arg_t args = *(struct arg_t*)arg;

	int serfd = args.serfd;


	int epoll_fd = epoll_create(100);
	
	struct epoll_event ev;
    struct epoll_event evs[ MAX_EVENT_NUM ];

	if( set_non_blocking(epoll_fd) == -1) {
		perror("epoll set non blocking ");
	}

	ev.events = EPOLLIN | EPOLLET;
	ev.data.fd = serfd;
	epoll_ctl(epoll_fd, EPOLL_CTL_ADD, serfd, &ev);

	int evnum = 0;
	int tempfd;
	struct epoll_event tempev;


	for(;;) {
		evnum = epoll_wait(epoll_fd, evs, MAX_EVENT_NUM, 10);
		// printf("%d\n", evnum);
		if(evnum == -1){
			perror("epoll wait");
			continue;
		}
			
		for(int i=0; i<evnum; i++) {
			
			if ((evs[i].events & EPOLLHUP)||(evs[i].events & EPOLLERR)) {
				printf("------------------------\n");
				if( epoll_ctl(epoll_fd, EPOLL_CTL_DEL, evs[i].data.fd, NULL) != -1){
					close(evs[i].data.fd);
				} else {
					perror("epoll event error");
				}
			} else if( evs[i].data.fd == serfd ) {
				handle_accept(serfd, epoll_fd);
				// addTask(threadPool1, serfd, epoll_fd, 1);

			} else if( evs[i].events & EPOLLIN ) {
				handle_read(evs[i].data.fd, epoll_fd);
				// addTask(threadPool1, evs[i].data.fd, epoll_fd, 2);

			} else if( evs[i].events & EPOLLOUT ) {
				handle_write(evs[i].data.fd, epoll_fd);
				// addTask(threadPool1, evs[i].data.fd, epoll_fd, 3);

			} else {
				printf("unknow events\n");
			}
		}
	}
	printf("--------------------------\n");
	close(serfd);
	close(epoll_fd);
}

void daemonize() {

    master = fork();

    if (master < 0) { 
        perror("fork"); exit(1); 
    } else if (master > 0) {

        // father exit
		printf("%d\n", master);
        exit(0);
    }
    
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

void check_and_restart(int serfd) {

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

					struct arg_t args;
					args.serfd = serfd;
					server_make((void*)(&args));

                    
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

void signal_handle(int signum) {

    if(signum == SIGUSR1) {
        server_running_flag = false;
        for(int i=0; i < WORKER_NUM; i++)
            kill(worker[i], SIGTERM);
    }
}


void dmf_server_show_info() {

	printf("Dmfserver Moule version:0.0.2\n\n");
	printf("--daemon running  PID: %d\n", master);


	printf("|--------SERVER CONFIGURE--------\n");
	printf("|PORT:%d\n", SERVER_PORT);
	printf("|MAX_EVENT:%d\n", MAX_EVENT_NUM);

	for(int i=0; i < WORKER_NUM; i++)
		printf("--worker nums:%d, pid:%d\n", i, worker[i]);

}



void start_server() {
	int serfd = create_socket( SERVER_PORT );

	struct arg_t args;

	args.serfd = serfd;

	server_make((void*)(&args));
	// for (int k = 0; k < 1; ++k) {
	// 	pthread_t roundCheck;
	// 	pthread_create(&roundCheck, NULL, server_make, (void*)(&args));
	// 	pthread_join(roundCheck, NULL);
	// }
}


// /*


int main(int arg, char* args[]) {

	// register signal  kill -10 pid
    signal(SIGUSR1, signal_handle);
	// printf("Now daemonize...\n");
	// daemonize();
	int serfd = create_socket( SERVER_PORT );
// -------------------------------------------------------------
	pid_t pid;

    for(int i=0; i < WORKER_NUM; i++) {
        // start worker process
        pid = fork();
        if (pid == 0) {

			struct arg_t args;
			// initThreadPool(&threadPool1, 1);
			args.serfd = serfd;

			server_make((void*)(&args));
			// for (int k = 0; k < 1; ++k) {
			// 	pthread_t roundCheck;
			// 	pthread_create(&roundCheck, NULL, server_make, (void*)(&args));
			// 	pthread_join(roundCheck, NULL);
			// }

        } else if (pid < 0){
            perror("fork failed!");
        } else {
            // father process
            worker[i] = pid;
        }
    }

	// show DMFserver basic confgure
	dmf_server_show_info();

	check_and_restart(serfd);

	return 0;
}

// */
