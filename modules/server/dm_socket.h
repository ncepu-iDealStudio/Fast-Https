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

#ifndef __DM_SOCKET_INCLUDE__
#define __DM_SOCKET_INCLUDE__

#include <sys/socket.h>
#include <netinet/in.h>         //for addr.sin_addr.s_addr = INADDR_ANY;
#include <sys/time.h>           //for struct timeval
#include <fcntl.h>              //for fcntl()
#include <stdio.h>              //perror
#include <string.h>
#include <arpa/inet.h>       // inet_ntoa

#ifdef __cplusplus
extern "C" {
#endif


extern int  set_non_blocking(int);
extern int  set_reuse(int);
extern int  create_socket(int);
extern int  client_socket();

#ifdef __cplusplus
}		/* end of the 'extern "C"' block */
#endif


#endif  // __DM_SOCKET_INCLUDE__

