#!/usr/bin/env python
import glob
import subprocess

files = []
# All go files in these directories will be run through gofmt
for p in ['*.go', 'cmd/dbmate/*.go']:
    files.extend(glob.glob(p))


exit_status = 0
for filename in files:
    if filename.endswith('.go'):
        try:
            out = subprocess.check_output(['gofmt', '-s', '-l', filename])
            if out != '':
                print out,
                exit_status = 1
        except subprocess.CalledProcessError:
            exit_status = 1

if exit_status != 0:
    print 'Reformat the files listed above with "gofmt -s -w" and try again.'
    exit(exit_status)

print 'All files pass gofmt.'
exit(0)
