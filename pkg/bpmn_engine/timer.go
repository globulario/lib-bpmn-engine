package bpmn_engine

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/globulario/lib-bpmn-engine/pkg/spec/BPMN20"
	"github.com/senseyeio/duration"
)

// Timer is created, when a process instance reaches a Timer Intermediate Message Event.
// The logic is simple: CreatedAt + Duration = DueAt
// The TimerState is one of [ TimerCreated, TimerTriggered, TimerCancelled ]
type Timer struct {
	ElementId          string        `json:"id"`
	ElementInstanceKey int64         `json:"ik"`
	ProcessKey         int64         `json:"pk"`
	ProcessInstanceKey int64         `json:"pik"`
	TimerState         TimerState    `json:"s"`
	CreatedAt          time.Time     `json:"c"`
	DueAt              time.Time     `json:"da"`
	Duration           time.Duration `json:"du"`
	// isBoundary marks timers created from boundaryEvent elements
	isBoundary      bool
	// parentElementId is the ID of the activity this boundary timer is attached to
	parentElementId string
	originActivity  activity
	baseElement     *BPMN20.BaseElement
}

type TimerState string

const TimerCreated TimerState = "CREATED"
const TimerTriggered TimerState = "TRIGGERED"
const TimerCancelled TimerState = "CANCELLED"

func (t Timer) Key() int64 {
	return t.ElementInstanceKey
}

func (t Timer) State() ActivityState {
	switch t.TimerState {
	case TimerCreated:
		return Active
	case TimerTriggered:
		return Completed
	case TimerCancelled:
		return Withdrawn
	}
	panic(fmt.Sprintf("[invariant check] missing mapping for timer state=%s", t.TimerState))
}

func (t *Timer) SetState(state ActivityState) {
	switch state {
	case Active:
		t.TimerState = TimerCreated
	case Completed:
		t.TimerState = TimerTriggered
	case Withdrawn:
		t.TimerState = TimerCancelled
	default:
		panic(fmt.Sprintf("[invariant check] missing mapping for timer state=%s", t.TimerState))
	}
}

func (t Timer) Element() *BPMN20.BaseElement {
	return t.baseElement
}

func (state *BpmnEngineState) handleIntermediateTimerCatchEvent(instance *processInstanceInfo, ice BPMN20.TIntermediateCatchEvent, originActivity activity) (continueFlow bool, timer *Timer, err error) {
	timer = findExistingTimerNotYetTriggered(state, ice.Id, instance)

	if timer != nil && timer.originActivity != nil {
		originActivity := instance.findActivity(timer.originActivity.Key())
		if originActivity != nil && (*originActivity.Element()).GetType() == BPMN20.EventBasedGateway {
			ebgActivity := originActivity.(*eventBasedGatewayActivity)
			if ebgActivity.OutboundCompleted() {
				timer.TimerState = TimerCancelled
				return false, timer, err
			}
		}
	}

	if timer == nil {
		timer, err = state.createTimer(instance, ice, originActivity)
		if err != nil {
			evalErr := &ExpressionEvaluationError{
				Msg: fmt.Sprintf("Error evaluating expression in intermediate timer cacht event element id='%s' name='%s'", ice.Id, ice.Name),
				Err: err,
			}
			return false, timer, evalErr
		}
	}

	if time.Now().After(timer.DueAt) {
		timer.TimerState = TimerTriggered
		if timer.originActivity != nil {
			originActivity := instance.findActivity(timer.originActivity.Key())
			if originActivity != nil && (*originActivity.Element()).GetType() == BPMN20.EventBasedGateway {
				ebgActivity := originActivity.(*eventBasedGatewayActivity)
				ebgActivity.SetOutboundCompleted(ice.Id)
			}
		}
		return true, timer, err
	}
	return false, timer, err
}

func (state *BpmnEngineState) createTimer(instance *processInstanceInfo, ice BPMN20.TIntermediateCatchEvent, originActivity activity) (*Timer, error) {
	variableContext := instance.VariableHolder.Variables()
	durationVal, err := findDurationValue(ice, variableContext)
	if err != nil {
		return nil, &BpmnEngineError{Msg: fmt.Sprintf("Error parsing 'timeDuration' value "+
			"from element with ID=%s. Error:%s", ice.Id, err.Error())}
	}
	var be BPMN20.BaseElement = ice
	now := time.Now()
	t := &Timer{
		ElementId:          ice.Id,
		ElementInstanceKey: state.generateKey(),
		ProcessKey:         instance.ProcessInfo.ProcessKey,
		ProcessInstanceKey: instance.InstanceKey,
		TimerState:         TimerCreated,
		CreatedAt:          now,
		DueAt:              durationVal.Shift(now),
		Duration:           time.Duration(durationVal.TS) * time.Second,
		baseElement:        &be,
		originActivity:     originActivity,
	}
	state.timers = append(state.timers, t)
	return t, nil
}

func findExistingTimerNotYetTriggered(state *BpmnEngineState, id string, instance *processInstanceInfo) *Timer {
	var t *Timer
	for _, timer := range state.timers {
		if timer.ElementId == id && timer.ProcessInstanceKey == instance.GetInstanceKey() && timer.TimerState == TimerCreated {
			t = timer
			break
		}
	}
	return t
}

func (state *BpmnEngineState) createBoundaryTimer(instance *processInstanceInfo, be BPMN20.TBoundaryEvent, parentActivity activity) (*Timer, error) {
	variableContext := instance.VariableHolder.Variables()
	durationVal, err := parseDurationText(be.TimerEventDefinition.TimeDuration.XMLText, be.Id, be.Name, variableContext)
	if err != nil {
		return nil, err
	}
	var elem BPMN20.BaseElement = be
	now := time.Now()
	t := &Timer{
		ElementId:          be.Id,
		ElementInstanceKey: state.generateKey(),
		ProcessKey:         instance.ProcessInfo.ProcessKey,
		ProcessInstanceKey: instance.InstanceKey,
		TimerState:         TimerCreated,
		CreatedAt:          now,
		DueAt:              durationVal.Shift(now),
		Duration:           time.Duration(durationVal.TS) * time.Second,
		isBoundary:         true,
		parentElementId:    be.AttachedToRef,
		baseElement:        &elem,
		originActivity:     parentActivity,
	}
	state.timers = append(state.timers, t)
	return t, nil
}

func findExistingBoundaryTimerNotYetTriggered(state *BpmnEngineState, boundaryEventId string, instance *processInstanceInfo) *Timer {
	for _, t := range state.timers {
		if t.isBoundary && t.ElementId == boundaryEventId && t.ProcessInstanceKey == instance.InstanceKey && t.TimerState == TimerCreated {
			return t
		}
	}
	return nil
}

func parseDurationText(durationStr string, elementId string, elementName string, variableContext map[string]interface{}) (duration.Duration, error) {
	if strings.HasPrefix(durationStr, "=") {
		v, err := evaluateExpression(durationStr, variableContext)
		if err != nil {
			return duration.Duration{}, &ExpressionEvaluationError{
				Msg: fmt.Sprintf("Error evaluating expression for timer id='%s' name='%s'", elementId, elementName),
				Err: err,
			}
		}
		if dur, ok := v.(string); ok {
			durationStr = dur
		} else {
			return duration.Duration{}, &ExpressionEvaluationError{
				Msg: fmt.Sprintf("Expression evaluated to invalid type for timer id='%s' name='%s'", elementId, elementName),
				Err: errors.New("expression evaluated to an invalid type"),
			}
		}
	}
	if len(strings.TrimSpace(durationStr)) == 0 {
		return duration.Duration{}, newEngineErrorf("Can't find 'timeDuration' value for element with id=%s", elementId)
	}
	return duration.ParseISO8601(durationStr)
}

func findDurationValue(ice BPMN20.TIntermediateCatchEvent, variableContext map[string]interface{}) (duration.Duration, error) {
	return parseDurationText(ice.TimerEventDefinition.TimeDuration.XMLText, ice.Id, ice.Name, variableContext)
}
