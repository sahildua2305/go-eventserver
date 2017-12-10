#! /bin/bash
( cd $(dirname $0)
time java -server -Xmx1G -jar ./follower-maze-2.0.jar)
