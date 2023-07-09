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

#include <dm_server.h>


/* test timer function start */
static void print_current_time() {
    time_t current_time = time(NULL);
    printf("Current time: %s", ctime(&current_time));
}



void* server_make(void* arg) {
	
	struct arg_t	    args         = *(struct arg_t*)arg;

	lis_inf_t 	  *		lis_infs	 = args.lis_infs;			// listen info array
	int 				lis_num      = args.lis_num;			// listen info num
	thread_pool_t * 	threadPool1  = args.ptr_thread_pool;


	int epoll_fd = epoll_create(100);
	
	struct epoll_event ev;
    struct epoll_event evs[ EPOLL_MAX_EVENT_NUM ];

#ifdef EPOLL_FD_NON_BLOCKING
	if( set_non_blocking(epoll_fd) == -1) {
		perror("epoll set non blocking ");
	}
#endif // EPOLL_FD_NON_BLOCKING

	// event.data.ptr
/*
	per_req_event_t* per_req_cli = (per_req_event_t*)malloc(sizeof(per_req_event_t));
	memset(per_req_cli, 0, sizeof(per_req_event_t));
	per_req_cli->fd = serfd;

	ev.events = EPOLLIN | EPOLLET;
	ev.data.ptr = (void*)per_req_cli;
	if( epoll_ctl(epoll_fd, EPOLL_CTL_ADD, serfd, &ev) == -1) {
		perror("epoll_ctl error");
	}
*/

	// event.data.fd

	for(int k=0; k<lis_num; k++) {
		ev.events = EPOLLIN | EPOLLET;
		ev.data.u32 = k;
		if( epoll_ctl(epoll_fd, EPOLL_CTL_ADD, lis_infs[k].fd, &ev) == -1) {
			perror("epoll_ctl error");
		}
	}
	
	
	int evnum = 0;
	int tempfd;
	uint32_t tempev;

	timer_min_heap_t* heap = (timer_min_heap_t*)malloc(sizeof(timer_min_heap_t));
	heap->size = 0;
	
	for(;;) {
		evnum = epoll_wait(epoll_fd, evs, EPOLL_MAX_EVENT_NUM, EPOLL_WAIT_TIMEOUT);
		// printf("_______%d______\n", getpid());
		if(evnum == -1) {
			perror("epoll wait");
			continue;
		}
		for(int i=0; i<evnum; i++) {
			// event.data.ptr
			// per_req_event_t* per_req_cli = (per_req_event_t*)(evs[i].data.ptr);
			tempfd = evs[i].data.fd;
			tempev = evs[i].events;

			if ( tempfd <= lis_num ) {

				handle_accept(lis_infs[tempfd].fd, epoll_fd);
			} else if( tempev & EPOLLIN ) {

				handle_read(tempfd, epoll_fd);
			} else if( tempev & EPOLLOUT ) {

				handle_write(tempfd, epoll_fd);
			} else if (( tempev & EPOLLHUP) || 
					   (tempev & EPOLLERR )) {

				printf("------------------------\n");
				handle_close(tempfd, epoll_fd);
			} else {

				printf("unknow events\n");
			}
		}
		handle_events(heap);
	}

	for(int k=0; k<lis_num; k++)
		close(lis_infs[k].fd);
	close(epoll_fd);
	free(heap);
}


void dmf_server_show_info() {

	printf("Dmfserver Moule version:0.0.2\n\n");
	printf("|--------SERVER CONFIGURE--------\n");
	printf("|MAX_EVENT:%d\n", EPOLL_WAIT_TIMEOUT);

}


void start_server(lis_inf_t *infs, int lis_num) {

	struct arg_t args;
	thread_pool_t threadPool1;
	// thread_pool_init(&threadPool1, 2);
	args.lis_infs = infs;
	args.ptr_thread_pool = &threadPool1;
	args.lis_num = lis_num;

	server_make((void*)(&args));

}


void start_multi_threading_server(lis_inf_t *infs, int lis_num) {

	struct arg_t args;
	thread_pool_t threadPool1;
	// thread_pool_init(&threadPool1, 2);
	args.lis_infs = infs;
	args.ptr_thread_pool = &threadPool1;
	args.lis_num = lis_num;

	server_make((void*)(&args));
	for (int k = 0; k < 1; ++k) {
		pthread_t roundCheck;
		pthread_create(&roundCheck, NULL, server_make, (void*)(&args));
		pthread_join(roundCheck, NULL);
	}
}

static SSL_CTX* get_ssl_ctx()
{
    SSL_CTX *ctx ;
    SSL_library_init();
    OpenSSL_add_all_algorithms();
    SSL_load_error_strings();
    ctx = SSL_CTX_new(SSLv23_server_method());
    if (ctx == NULL) {
        ERR_print_errors_fp(stdout);
        return NULL;
    }
    if (SSL_CTX_use_certificate_file(ctx, "./localhost.pem" , SSL_FILETYPE_PEM) <= 0) {
        ERR_print_errors_fp(stdout);
        return NULL;
    }
    if (SSL_CTX_use_PrivateKey_file(ctx, "./localhost-key.pem" , SSL_FILETYPE_PEM) <= 0) {
        ERR_print_errors_fp(stdout);
        return NULL;
    }
    if (!SSL_CTX_check_private_key(ctx)) {
        ERR_print_errors_fp(stdout);
        return NULL;
    }
    return ctx;
}

extern int epoll_ssl_server(int serfd) {

	/*创建ssl上下文*/
	SSL_CTX *ctx = get_ssl_ctx();
	if(ctx == NULL){
		return 1;
	}

	printf("ssl load ok\n");
	
	int efd = epoll_create(100);
	assert(efd > 0);
	printf("epoll fd %d\n", efd);

	struct epoll_event events[100];
	struct epoll_event ev;
	ev.data.fd = serfd;
	ev.events = EPOLLET | EPOLLIN;
	epoll_ctl(efd, EPOLL_CTL_ADD, serfd, &ev);


	fd_ssl_map* fsm_head = (fd_ssl_map*)malloc(sizeof(fd_ssl_map));
	fsm_head->next = NULL;

	
	printf("server is listening...\n");
	static const char *https_response = 
		"HTTP/1.1 200 OK\r\nServer: httpd\r\nContent-Length: %d\r\nConnection: keep-alive\r\n\r\n";

	while (true) {
		// printf("epoll wait...\n");
		int nev = epoll_wait(efd, events, sizeof(events) / sizeof(struct epoll_event), -1);
		if (nev < 0) {
			printf("epoll_wait error. [%d,%s]", errno, strerror(errno));
			break;
		}

		for (size_t i = 0; i < nev; ++i) {
			// auto &event = events[i];
			struct epoll_event event = events[i];

			if (event.data.fd == serfd) {  // accept
				struct sockaddr_in addr;
				socklen_t len = sizeof(addr);
				int cfd = accept(serfd, (struct sockaddr *)&addr, &len);
				if (cfd > 0) {
					printf("accept client %d [%s:%d]\n", cfd, inet_ntoa(addr.sin_addr), ntohs(addr.sin_port));
					SSL *ssl = SSL_new(ctx);
					bool isSSLAccept = true;
					if (ssl == NULL) {
						printf("SSL_new error.\n");
						continue;
					}
					int flags = fcntl(cfd, F_GETFL, 0);
					fcntl(cfd, F_SETFL, flags | O_NONBLOCK);

					SSL_set_fd(ssl, cfd);
					int code;
					int retryTimes = 0;

					while ((code = SSL_accept(ssl)) <= 0 && retryTimes++ < 100) {
						if (SSL_get_error(ssl, code) != SSL_ERROR_WANT_READ) {
							printf("ssl accept error. %d\n", SSL_get_error(ssl, code));
							break;
						}
						usleep(1);
						printf("-------|");
					}

					printf("code %d, retry times %d\n", code, retryTimes);
					if (code != 1) {
						isSSLAccept = false;
						close(cfd);
						SSL_free(ssl);
						continue;
					}
					ev.data.fd = cfd;
					ev.events = EPOLLET | EPOLLIN;
					epoll_ctl(efd, EPOLL_CTL_ADD, cfd, &ev);
					
					fd_ssl_map* per = (fd_ssl_map*)malloc(sizeof(fd_ssl_map));
					per->fd = cfd; per->ssl = ssl; per->next = NULL;
					if(fsm_head->next == NULL)
						fsm_head->next = per;
					else{
						per->next = fsm_head->next;
						fsm_head->next = per;
					}

				} else {
					perror("accept error");
				}
				continue;
			}

			fd_ssl_map* it = fsm_head;
			fd_ssl_map* pre;

			while(it->next != NULL) {
				if(it->fd == event.data.fd)
					break;
				pre = it;
				it = it->next;
			}

			if (event.events & (EPOLLRDHUP | EPOLLHUP)) {
				printf("client %d quit!\n", event.data.fd);
				close(event.data.fd);
				SSL_shutdown(it->ssl);
				SSL_free(it->ssl);
				
				if(it->next == NULL)
					pre->next = NULL;
				pre->next = it->next;


				epoll_ctl(efd, EPOLL_CTL_DEL, event.data.fd, NULL);
				continue;
			}

			if (event.events & EPOLLIN) {
				char buf[512] = {0};
				int readSize = SSL_read(it->ssl, buf, sizeof(buf));
				if (readSize <= 0) {
					printf("SSL_read error. %d\n", SSL_get_error(it->ssl, readSize));
					continue;
				}
				printf("read: %d\n%s\n", readSize, buf);

				char sendBuf[1024] = {0};
				int fmtSize = sprintf(sendBuf, https_response, readSize);

				printf("*********************\n%s*********************\n", sendBuf);
				int writeSize = SSL_write(it->ssl, sendBuf, strlen(sendBuf));    // 发送响应头
				printf("format size %d, write size %d\n", fmtSize, writeSize);
				if (writeSize <= 0) {
					printf("SSL_write error. %d\n", SSL_get_error(it->ssl, writeSize));
				}
				writeSize = SSL_write(it->ssl, buf, readSize);   // 发送响应主体
				if (writeSize <= 0) {
					printf("SSL_write error. %d\n", SSL_get_error(it->ssl, writeSize));
				}
				printf("format size %d, write size %d\n", fmtSize, writeSize);
			}
		}
	}

	fd_ssl_map* it = fsm_head;
	while(it->next != NULL){
		close(it->fd);
		SSL_free(it->ssl);
	}
	

	SSL_CTX_free(ctx);
	close(serfd);
	close(efd);
	return 0;
}
