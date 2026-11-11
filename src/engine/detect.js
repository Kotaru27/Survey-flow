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
// A test navigator or survey debug panel can contain terms like "screen out",
// "terminated" and "not eligible" even while the respondent has an ordinary Next
// button. When present, the document's main region is the respondent-facing text.
// Do not classify an active question/information page as an end page from a
// free-text match; URL-verified end pages remain terminal regardless of buttons.
const respondentRegion=document.querySelector('main,[role="main"]');
const respondentText=norm((respondentRegion&&respondentRegion.innerText)||body).toLowerCase();
const proceedRe=/^(next|continue|start|begin|proceed|submit|finish|go to next|>|>>|ÃƒÂ¢Ã¢â‚¬Â Ã¢â‚¬â„¢)$/i;
const hasProceed=[...clickable,...native.filter(e=>/^(button|link|menuitem)$/i.test(e.getAttribute?.('role')||''))].some(e=>
 !e.disabled && [txt(e),e.value,e.getAttribute?.('aria-label'),e.getAttribute?.('title')].some(v=>proceedRe.test(norm(v||''))));
const screenText=/profile does not correspond|does not correspond to the one|you (do not|don't) (qualify|match)|does not qualify|not qualified|not eligible|ineligible|unfortunately[\s\S]{0,140}(qualify|correspond|eligible|profile)/i.test(respondentText);
// A bare "screenout", "terminated" or "disqualified" field name is metadata,
// not a respondent-visible outcome. Explicit rejection wording is required.
const explicitEndText=screenText||/quota (?:is )?(?:full|filled)|you (?:have been|were) screened out|survey is closed|quota has been reached/i.test(respondentText);
const termination=endTerm||(!hasProceed&&explicitEndText);
const terminationEvidence=endTerm?'termination-url':termination?'respondent-message-no-next':'';
const returningSession=/[?&#]returning=(yes|true)/i.test(location.href)||/already (completed|participated|taken|finished)|duplicate (test )?id|this (id|code) (was|has been) (already )?used|id (is )?(already|in use)/i.test(respondentText);
const openLeft=textInputs.filter(e=>!e.disabled&&!e.value).length;
const workLeft=nativeRadios.length+roleRadios.length+nativeChecks.length+roleChecks.length+selects.length+openLeft;
const thanksPage=/(thank you( very much)? for your time|thank you for (taking|completing|participating|sharing)|thanks for (taking|completing)|we appreciate your (time|participation))/.test(lower);
const terminal=thanksPage&&workLeft===0&&!hasProceed;
const answered=__ANSWERED__;
// Thank-you or generic 'submitted' text is not sufficient proof of a qualify.
const completion=!termination&&endQual;
