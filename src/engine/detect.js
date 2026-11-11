// Read explicit validation messages even when the survey links them through ARIA
// rather than rendering a visible .error / role=alert element.
function readErrs(){
 const nodes=[...document.querySelectorAll('[role="alert"],[aria-live="assertive"],.error,.errors,.validation-error,[class*="error"],[class*="Error"],[class*="validation"],[class*="Validation"]')];
 for(const field of document.querySelectorAll('[aria-invalid="true"]')){
  const ids=field.getAttribute('aria-errormessage')||field.getAttribute('aria-describedby')||'';
  for(const id of ids.trim().split(/\s+/)){
   if(!id)continue;
   const node=document.getElementById(id);
   if(node)nodes.push(node);
  }
 }
 return [...new Set(nodes.filter(vis).map(txt).map(norm).filter(Boolean))].slice(0,12);
}
const valErr=readErrs();
const needMore=/please select|please choose|please tick|at least (one|two|three|1|2|3)|select at least|choose at least|tick at least|must select|must choose/i.test(valErr.join(' '));
// A step can land while the document has no body yet - the survey navigated underneath us
// and the parser has not built one. Reading document.body.innerText then throws, and the
// throw takes the whole step with it: the page is never answered and never reported.
const body=norm((document.body&&document.body.innerText)||'');
const lower=body.toLowerCase();
// Toluna/Confirmit end pages live under BCEndPages/<language>-Qual.html (real submission) or
// BCEndPages/<language>-Term.html (screen-out / termination). Both of them open with
// "Thank you for sharing your opinions!" and carry "Only one submission per person is
// accepted", so neither of those strings may be used as a completion signal.
const endPage=/\/BCEndPages\//i.test(location.href);
// Qualification is confirmed only by the explicit survey qualification end URL.
const endQual=/(?:^|[\/-])Qual\.html(?:[?#]|$)/i.test(location.href);
const endTerm=endPage&&/-Term\b/i.test(location.href);
const screenText=/profile does not correspond|does not correspond to the one|you (do not|don't) (qualify|match)|does not qualify|not qualified|not eligible|ineligible|unfortunately[\s\S]{0,140}(qualify|correspond|eligible|profile)/i.test(lower);
const termination=endTerm||screenText||/quota (full|filled)|screen ?out|you do not qualify|not qualify|terminated|disqualified|survey is closed|quota has been reached/.test(lower);
const returningSession=/[?&#]returning=(yes|true)/i.test(location.href)||/already (completed|participated|taken|finished)|duplicate (test )?id|this (id|code) (was|has been) (already )?used|id (is )?(already|in use)/i.test(body);
const openLeft=textInputs.filter(e=>!e.disabled&&!e.value).length;
const workLeft=nativeRadios.length+roleRadios.length+nativeChecks.length+roleChecks.length+selects.length+openLeft;
const proceedRe=/^(next|continue|start|begin|proceed|submit|finish|go to next|>|>>|ÃƒÂ¢Ã¢â‚¬Â Ã¢â‚¬â„¢)$/i;
const hasProceed=[...clickable,...native.filter(e=>/^(button|link|menuitem)$/i.test(e.getAttribute?.('role')||''))].some(e=>proceedRe.test(txt(e))&&!e.disabled);
const thanksPage=/(thank you( very much)? for your time|thank you for (taking|completing|participating|sharing)|thanks for (taking|completing)|we appreciate your (time|participation))/.test(lower);
const terminal=thanksPage&&workLeft===0&&!hasProceed;
const answered=__ANSWERED__;
// Thank-you or generic 'submitted' text is not sufficient proof of a qualify.
const completion=!termination&&endQual;
