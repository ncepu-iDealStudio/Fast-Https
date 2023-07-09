#CMake最低版本号要求
cmake_minimum_required(VERSION 3.10.0)

#指定项目名称
project(DmfServer)

set(CMAKE_SYSTEM_VERSION 1)

find_package(Threads REQUIRED)
find_package(OpenSSL REQUIRED)

link_libraries(pthread ssl crypto)

file( GLOB main_SRC "*.h" "*.c" )
include_directories(${PROJECT_SOURCE_DIR})

add_executable(dmfserver ${main_SRC} )