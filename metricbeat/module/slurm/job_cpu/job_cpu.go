package job_cpu

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strings"
	"strconv"

	"github.com/elastic/elastic-agent-libs/mapstr"
	"github.com/elastic/beats/v7/metricbeat/mb"
	"github.com/mitchellh/go-ps"
)

var slurmdir string
var steps bool

// init registers the MetricSet with the central registry as soon as the program
// starts. The New function will be called later to instantiate an instance of
// the MetricSet for each host defined in the module's configuration. After the
// MetricSet has been created then Fetch will begin to be called periodically.
func init() {
	mb.Registry.MustAddMetricSet("slurm", "job_cpu", New)
}

// MetricSet holds any configuration or state information. It must implement
// the mb.MetricSet interface. And this is best achieved by embedding
// mb.BaseMetricSet because it implements all of the required mb.MetricSet
// interface methods except for Fetch.
type MetricSet struct {
	mb.BaseMetricSet
	job_user string
	jobid int
	step string
	cpuutil float64
	cpuused float64
	cpureq float64
}

// New creates a new instance of the MetricSet. New is responsible for unpacking
// any MetricSet specific configuration options if there are any.
func New(base mb.BaseMetricSet) (mb.MetricSet, error) {
	slurmdir = "/sys/fs/cgroup/cpuset/slurm"

	config := struct{}{}
	if err := base.Module().UnpackConfig(&config); err != nil {
		return nil, err
	}

	return &MetricSet{
		BaseMetricSet: base,
		job_user: "",
		jobid: -1,
		step: "",
		cpuutil: -1.0,
		cpuused: -1.0,
		cpureq: -1.0,
	}, nil
}

// Fetch methods implements the data gathering and data conversion to the right
// format. It publishes the event which is then forwarded to the output. In case
// of an error set the Error field of mb.Event or simply call report.Error().
func (m *MetricSet) Fetch(report mb.ReporterV2) error {

	lscpu_out, err := exec.Command("lscpu").Output()
	if err != nil {
		return fmt.Errorf("failed to get lscpu output: %w", err)
	}
	threads_num, err := strconv.Atoi(strings.TrimSpace(strings.Split(strings.Split(string(lscpu_out), "\n")[5], ":")[1]))
	if err != nil {
		return fmt.Errorf("failed to get the number of threads per core: %w", err)
	}

	allproc, err := ps.Processes()
	if err != nil {
		return fmt.Errorf("failed to get processes: %w", err)
	}

	for _, p := range allproc {
		if p.Executable()=="slurmstepd" {
			pid := p.Pid()
			pid_str := strconv.Itoa(pid)
			proc_status, err := os.ReadFile("/proc/"+pid_str+"/status")
			if err != nil {
				m.Logger().Errorf("failed to get /proc status info: %s", err)
				continue
			}
			job_uid := strings.Split(strings.Split(string(proc_status), "\n")[8], "\t")[1]
			job_user_info, err := user.LookupId(job_uid)
			if err != nil {
				m.Logger().Errorf("failed to get username: %s", err)
			} else {
				m.job_user = job_user_info.Username
			}
			fullcom, err := os.ReadFile("/proc/"+pid_str+"/cmdline")
			if err != nil {
				m.Logger().Errorf("failed to get full process command: %s", err)
			}
			job_step := strings.Split(strings.Split(string(fullcom), "[")[1], ".")
			m.jobid, err = strconv.Atoi(job_step[0])
			if err != nil {
				m.Logger().Errorf("failed to convert jobid into int: %s", err)
				continue
			}
			m.step = strings.TrimSuffix(job_step[1], "]")
			curr_step_dir := slurmdir + "/uid_" + job_uid + "/job_" + job_step[0] + "/step_" + m.step + "/"
			cpus_val, err := os.ReadFile(curr_step_dir + "cpuset.cpus")
			if err != nil {
				m.Logger().Errorf("failed to read cpuset.cpus: %s", err)
				continue
			}
			cpus := 0
			cpus_val_slice := strings.Split(string(cpus_val), ",")
			for _, val := range cpus_val_slice {
				cpus_val_range_slice := strings.Split(val, "-")
				if len(cpus_val_range_slice) == 2 {
					first_num, err := strconv.Atoi(strings.TrimSpace(cpus_val_range_slice[0]))
					if err != nil {
						m.Logger().Errorf("failed to convert used cpu number into int: %s", err)
						continue
					}
					second_num, err := strconv.Atoi(strings.TrimSpace(cpus_val_range_slice[1]))
					if err != nil {
						m.Logger().Errorf("failed to convert used cpu number into int: %s", err)
						continue
					}
					cpus += second_num - first_num + 1
				} else if len(cpus_val_range_slice) == 1 {
					cpus += 1
				}
			}
			m.cpureq = float64(cpus / threads_num)
			cpuutil_calc := 0.0
			stat, err := os.ReadFile("/proc/"+pid_str+"/stat")
			if err != nil {
				m.Logger().Errorf("failed to get /proc stat info: %s", err)
				continue
			}
			sid := strings.Split(string(stat), " ")[5]
			outp_pcpu, err := exec.Command("ps", "-o", "pcpu=", "-g", sid).Output()
			if err != nil {
				m.Logger().Errorf("failed to get info on cpu utilization of processes belonging to session: %s", err)
				continue
			}
			outp_pcpu_slice := strings.Split(strings.TrimSpace(string(outp_pcpu)), "\n")
			for _, o := range outp_pcpu_slice {
				num, err := strconv.ParseFloat(strings.TrimSpace(o), 32)
				if err != nil {
					m.Logger().Errorf("failed to parse cpu utilization value as float: %s", err)
					continue
				}
				cpuutil_calc += num
			}
			m.cpuutil = cpuutil_calc / m.cpureq
			if m.cpuutil > 100.0 {
				m.cpuutil = 100.0
			}
			m.cpuused = m.cpuutil * m.cpureq / 100.0

			report.Event(mb.Event{
				ModuleFields: mapstr.M{
					"job_user": m.job_user,
					"jobid": m.jobid,
					"step": m.step,
				},
				MetricSetFields: mapstr.M{
					"cpuutil": m.cpuutil,
					"cpuused": m.cpuused,
					"cpureq": m.cpureq,
				},
			})
		}
	}

	return nil
}
