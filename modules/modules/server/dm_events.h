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

#ifndef __DM_EVENT_INCLUDE__
#define __DM_EVENT_INCLUDE__

#include <dm_server_config.h>
#include <dm_socket.h>          // set_nonblocking
// #include <dm_server.h>

#include <sys/epoll.h>
#include <unistd.h>             // for close
#include <stdio.h>              // perror
#include <sys/socket.h>
#include <errno.h>              // errno
#include <stdlib.h>
#include <string.h>
#include <openssl/ssl.h>
#include <openssl/err.h>

#ifdef __cplusplus
extern "C" {
#endif


typedef struct _per_req_event_s {

	int                         fd;
	lis_type_t 	                type;
	SSL                   *		ssl;
	void                  *     data;
} per_req_event_t;


extern void handle_accept (int serfd, int epoll_fd);
// void handle_accept_http ( int serfd, int epoll_fd );


extern void handle_read (int client_fd, int epoll_fd);
extern void handle_write (int client_fd, int epoll_fd);
extern void handle_shutdown (int client_fd, int epoll_fd, int how);
extern void handle_close (int client_fd, int epoll_fd);


#ifdef __cplusplus
}		/* end of the 'extern "C"' block */
#endif


#endif  // __DM_EVENT_INCLUDE__

