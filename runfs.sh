MOD=/usr/local/freeswitch/mod
[ -d "/usr/lib/freeswitch/mod" ] && MOD=/usr/lib/freeswitch/mod
echo "using mod:$MOD"
freeswitch -nonat -base ./freeswitch -mod $MOD