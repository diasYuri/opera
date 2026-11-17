import os,sys,json,time,subprocess,datetime,platform,collections
from pathlib import Path
mode=sys.argv[1]
root=Path('/Users/yuri/git/diasYuri/opera')
logs=root/'.agents/validation/macos-arm64-2026-11-16'
logs.mkdir(parents=True,exist_ok=True)
commands={
 'check':['go','run','./scripts','check'],
 'lint':['go','run','./scripts','lint'],
 'tests':['go','test','-count=1','-json','./...'],
 'race':['go','test','-race','-count=1','-json','./...'],
 'untagged':['go','test','-count=1','-json','./...'],
 'consumer':['go','-C','testdata/sdk-consumer','test','-count=1','-mod=readonly','-json','./...'],
 'live':['go','test','-count=1','-json','-run','^TestLiveVerticalProof$','./internal/harness'],
 'package_smoke':['go','run','./scripts','package-smoke'],
}
env=os.environ.copy();env.update(GOTOOLCHAIN='go1.27.4',GOWORK='off',CGO_ENABLED='1' if mode=='race' else '0',GOFLAGS='' if mode=='untagged' else '-tags=feature_lsp,feature_wasm_ext',OPERA_LIVE_TEST='1' if mode=='live' else '0')
if mode=='live' and not env.get('OPENAI_API_KEY'):
 result={'mode':mode,'status':'unavailable','reason':'OPENAI_API_KEY not configured'}
 (logs/(mode+'.summary.json')).write_text(json.dumps(result,indent=2)+'\n');print(json.dumps(result));sys.exit(0)
start=time.monotonic();started=datetime.datetime.now(datetime.timezone.utc).isoformat()
process=subprocess.run(commands[mode],cwd=root,env=env,capture_output=True,text=True,timeout=600)
output=process.stdout+process.stderr
secret=env.get('OPENAI_API_KEY','')
if secret:output=output.replace(secret,'[REDACTED]')
(logs/(mode+'.log')).write_text(output)
counts=collections.Counter();packages=[];failed=[];skipped=[];tests=[]
for line in output.splitlines():
 try:event=json.loads(line)
 except (ValueError,TypeError):continue
 action=event.get('Action');test=event.get('Test')
 if test and action in ['pass','fail','skip']:
  counts[action]+=1;tests.append({'package':event.get('Package'),'test':test,'status':action})
  if action=='fail':failed.append(test)
  if action=='skip':skipped.append(test)
 if not test and action in ['pass','fail','skip']:packages.append({'package':event.get('Package'),'status':action})
summary={'mode':mode,'command':commands[mode],'started_utc':started,'seconds':round(time.monotonic()-start,3),'exit_code':process.returncode,'platform':platform.system()+'/'+platform.machine(),'toolchain':'go1.27.4','cgo':env['CGO_ENABLED'],'tags':env['GOFLAGS'],'test_counts':dict(counts),'packages':packages,'failed':failed,'skipped':skipped}
(logs/(mode+'.summary.json')).write_text(json.dumps(summary,indent=2)+'\n')
(logs/(mode+'.tests.json')).write_text(json.dumps(tests,indent=2)+'\n')
print(json.dumps(summary))
if process.returncode:print(output[-10000:])
sys.exit(process.returncode)
