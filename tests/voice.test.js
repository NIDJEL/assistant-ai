import assert from 'node:assert/strict';
import test from 'node:test';
import {VoiceInput,speakText} from '../app/static/voice.js';

class Recognition {
  start(){this.onstart()}
  stop(){this.onend()}
  result(text,final=true){this.onresult({resultIndex:0,results:[Object.assign([{transcript:text}],{isFinal:final})]})}
}
function harness(Impl=Recognition){const calls={state:[],transcript:[],final:[],errors:[]};const voice=new VoiceInput({Recognition:Impl,onState:x=>calls.state.push(x),onTranscript:x=>calls.transcript.push(x),onFinal:x=>calls.final.push(x),onError:x=>calls.errors.push(x)});return {voice,calls}}
test('only final recognized speech is submitted, once',()=>{const {voice,calls}=harness();voice.toggle();voice.recognition.result('завтра',false);assert.deepEqual(calls.final,[]);voice.recognition.result('Напомни завтра позвонить Ивану');voice.toggle();voice.recognition.onend();assert.deepEqual(calls.final,['Напомни завтра позвонить Ивану']);assert.deepEqual(calls.state,[true,false,false]);assert.equal(voice.recognition.lang,'ru-RU')});
test('no speech and provisional speech do not create an action',()=>{const {voice,calls}=harness();voice.toggle();voice.recognition.result('встре',false);voice.toggle();assert.deepEqual(calls.final,[])});
test('microphone or network errors prevent a partial action',()=>{for(const error of ['not-allowed','network','audio-capture','no-speech']){const {voice,calls}=harness();voice.toggle();voice.recognition.result('Напомни завтра');voice.recognition.onerror({error});voice.toggle();assert.deepEqual(calls.final,[]);assert.deepEqual(calls.errors,[error])}});
test('a failed attempt can be retried successfully',()=>{const {voice,calls}=harness();voice.toggle();voice.recognition.onerror({error:'network'});voice.toggle();voice.toggle();voice.recognition.result('Что у меня сегодня');voice.toggle();assert.deepEqual(calls.final,['Что у меня сегодня'])});
test('unsupported browser offers text fallback',()=>{const {voice,calls}=harness(null);voice.toggle();assert.equal(voice.supported,false);assert.deepEqual(calls.errors,['unsupported'])});
test('speech synthesis selects Russian voice and limits long answers',()=>{const spoken=[];let canceled=0;class Utterance{constructor(text){this.text=text}}const russian={lang:'ru-RU'};speakText('x'.repeat(2000),{cancel:()=>canceled++,getVoices:()=>[{lang:'en-US'},russian],speak:u=>spoken.push(u)},Utterance);assert.equal(canceled,1);assert.equal(spoken[0].text.length,1200);assert.equal(spoken[0].voice,russian);assert.equal(spoken[0].lang,'ru-RU')});
