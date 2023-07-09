
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


#ifndef __DM_SERVER_INCLUDE__
#define __DM_SERVER_INCLUDE__

#include <dm_server_config.h>
#include <dm_threading_pool.h>
#include <dm_socket.h>        
#include <dm_events.h>
#include <dm_timer.h>
#include <arpa/inet.h>       // inet_ntoa

#include <openssl/ssl.h>
#include <openssl/err.h>
#include <assert.h>


// #define SERVER_PORT 8080
#define EPOLL_FD_NON_BLOCKING
#define EPOLL_MAX_EVENT_NUM 1024
#define EPOLL_WAIT_TIMEOUT 40


typedef struct _listen_info_s {
	char                        ip[64];
	int                         port;
	int                         fd;
	lis_type_t type;
} lis_inf_t;


struct arg_t {
	lis_inf_t             *    lis_infs;
    int                        lis_num;
	thread_pool_t         *    ptr_thread_pool;
};


typedef struct fd_ssl_map {
    int                         fd;
    SSL*                        ssl;
    struct fd_ssl_map       *   next;
} fd_ssl_map ;

#ifdef __cplusplus
extern "C" {
#endif

extern void* server_make(void *arg);
extern void  dmf_server_show_info();
extern void  start_server(lis_inf_t *infs, int lis_num);
extern void  start_multi_threading_server(lis_inf_t *infs, int lis_num);
extern int   epoll_ssl_server(int serfd); 

#ifdef __cplusplus
}		/* end of the 'extern "C"' block */
#endif


#endif  // __DM_SERVER_INCLUDE__